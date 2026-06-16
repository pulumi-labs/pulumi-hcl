// Copyright 2026, Pulumi Corporation.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// Package schema generates Pulumi package schemas from HCL module definitions.
package schema

import (
	"encoding/json"
	"fmt"
	"sort"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/zclconf/go-cty/cty"
)

// ModuleSchema represents a generated schema for an HCL module.
type ModuleSchema struct {
	// PackageName is the package name used in the schema and token prefix.
	PackageName string `json:"packageName"`

	// Version is the module version.
	Version string `json:"version,omitempty"`

	// ComponentName is the component name used in the token suffix.
	ComponentName string `json:"componentName"`

	// Module is the module segment used in the token middle.
	Module string `json:"module"`

	// Description is the module description.
	Description string `json:"description,omitempty"`

	// InputProperties are the module's input variables.
	InputProperties map[string]*PropertySpec `json:"inputProperties,omitempty"`

	// RequiredInputs lists required input property names.
	RequiredInputs []string `json:"requiredInputs,omitempty"`

	// OutputProperties are the module's outputs.
	OutputProperties map[string]*PropertySpec `json:"outputProperties,omitempty"`
}

// PropertySpec describes a property in the schema.
type PropertySpec struct {
	// Type is the property type.
	Type string `json:"type,omitempty"`

	// Description is the property description.
	Description string `json:"description,omitempty"`

	// Default is the default value, if any.
	Default any `json:"default,omitempty"`

	// Secret indicates if the property is secret.
	Secret bool `json:"secret,omitempty"`

	// Items describes array element types.
	Items *PropertySpec `json:"items,omitempty"`

	// AdditionalProperties describes map value types.
	AdditionalProperties *PropertySpec `json:"additionalProperties,omitempty"`

	// Properties describes object properties.
	Properties map[string]*PropertySpec `json:"properties,omitempty"`

	// Ref is a reference to another type definition.
	Ref string `json:"$ref,omitempty"`
}

// GenerateModuleSchema generates a Pulumi schema from an HCL module configuration.
func GenerateModuleSchema(config *ast.Config, pkgName, pkgVersion, componentName, module string) (*ModuleSchema, error) {
	schema := &ModuleSchema{
		PackageName:      pkgName,
		Version:          pkgVersion,
		ComponentName:    componentName,
		Module:           module,
		InputProperties:  make(map[string]*PropertySpec),
		OutputProperties: make(map[string]*PropertySpec),
	}

	for _, v := range config.Variables {
		prop, err := variableToPropertySpec(v)
		if err != nil {
			return nil, fmt.Errorf("processing variable %q: %w", v.Name, err)
		}
		schema.InputProperties[v.Name] = prop

		// Track required inputs (no default and not nullable)
		if v.Default == nil && !v.Nullable {
			schema.RequiredInputs = append(schema.RequiredInputs, v.Name)
		}
	}

	evaluator := eval.NewEvaluator(buildTypeScope(config))
	for _, o := range config.Outputs {
		prop, err := outputToPropertySpec(o, inferOutputType(evaluator, o))
		if err != nil {
			return nil, fmt.Errorf("processing output %q: %w", o.Name, err)
		}
		schema.OutputProperties[o.Name] = prop
	}

	sort.Strings(schema.RequiredInputs)

	return schema, nil
}

// buildTypeScope returns an evaluation context in which variable and local
// references resolve to unknown values of their inferred types, so that an
// output expression can be evaluated for its type alone. Type information rides
// through cty evaluation, so typing an output is just evaluating it against
// typed unknowns and reading the result type.
//
// Resource, data source, and module references are intentionally left unbound:
// typing them requires resolving provider schemas, which is not available here.
// An output referencing one fails to evaluate and falls back to the "any" type.
func buildTypeScope(config *ast.Config) *eval.Context {
	ctx := eval.NewContext(".", ".", ".", "", "", "")

	for name, v := range config.Variables {
		t := v.TypeConstraint
		if t == cty.NilType {
			t = cty.DynamicPseudoType
		}
		ctx.SetVariable(name, cty.UnknownVal(t))
	}

	// Locals may reference variables and one another. Evaluate to a fixpoint:
	// each pass types any local whose references already resolve. A local that
	// never resolves (a cycle, or a reference to something unbound) is left
	// unset, so an output using it falls back to "any".
	evaluator := eval.NewEvaluator(ctx)
	remaining := make(map[string]*ast.Local, len(config.Locals))
	for name, l := range config.Locals {
		remaining[name] = l
	}
	for len(remaining) > 0 {
		progress := false
		for name, l := range remaining {
			val, diags := evaluator.Evaluate(l.Value)
			if diags.HasErrors() {
				continue
			}
			ctx.SetLocal(name, cty.UnknownVal(val.Type()))
			delete(remaining, name)
			progress = true
		}
		if !progress {
			break
		}
	}

	return ctx
}

// inferOutputType evaluates an output's value expression against the type scope
// and returns its type, or cty.DynamicPseudoType (which maps to "any") when the
// expression cannot be evaluated for a type.
func inferOutputType(evaluator *eval.Evaluator, o *ast.Output) cty.Type {
	if o.Value == nil {
		return cty.DynamicPseudoType
	}
	val, diags := evaluator.Evaluate(o.Value)
	if diags.HasErrors() {
		return cty.DynamicPseudoType
	}
	return val.Type()
}

// variableToPropertySpec converts an HCL variable to a PropertySpec.
func variableToPropertySpec(v *ast.Variable) (*PropertySpec, error) {
	prop := &PropertySpec{
		Description: v.Description,
		Secret:      v.Sensitive,
	}

	// Convert type constraint to schema type
	if v.TypeConstraint != cty.NilType {
		typeSpec, err := ctyTypeToPropertySpec(v.TypeConstraint)
		if err != nil {
			return nil, err
		}
		prop.Type = typeSpec.Type
		prop.Items = typeSpec.Items
		prop.AdditionalProperties = typeSpec.AdditionalProperties
		prop.Properties = typeSpec.Properties
	} else {
		// Default to any type if no constraint specified
		prop.Type = "object"
	}

	// A variable default is a constant expression (it cannot reference other
	// values), so a nil evaluation context is sufficient. The Pulumi schema
	// only permits constant defaults on primitive properties; a default on a
	// collection or object is conveyed by the property being optional rather
	// than by an explicit default value.
	if v.Default != nil {
		val, diags := v.Default.Value(nil)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating default: %s", diags.Error())
		}
		if d, ok := ctyToConstant(val); ok {
			prop.Default = d
		}
	}

	return prop, nil
}

// ctyToConstant converts a primitive cty.Value to a Go value usable as a Pulumi
// schema default. Only booleans, numbers, and strings are valid schema
// constants, so null and non-primitive values return ok=false.
func ctyToConstant(val cty.Value) (any, bool) {
	if val.IsNull() {
		return nil, false
	}
	switch val.Type() {
	case cty.String:
		return val.AsString(), true
	case cty.Number:
		f, _ := val.AsBigFloat().Float64()
		return f, true
	case cty.Bool:
		return val.True(), true
	default:
		return nil, false
	}
}

// outputToPropertySpec converts an HCL output to a PropertySpec. typ is the type
// inferred from the output's value expression; a DynamicPseudoType maps to the
// "object" (any) type.
func outputToPropertySpec(o *ast.Output, typ cty.Type) (*PropertySpec, error) {
	prop, err := ctyTypeToPropertySpec(typ)
	if err != nil {
		return nil, err
	}
	prop.Description = o.Description
	prop.Secret = o.Sensitive
	return prop, nil
}

// ctyTypeToPropertySpec converts a cty.Type to a PropertySpec.
func ctyTypeToPropertySpec(t cty.Type) (*PropertySpec, error) {
	switch {
	case t == cty.String:
		return &PropertySpec{Type: "string"}, nil

	case t == cty.Number:
		return &PropertySpec{Type: "number"}, nil

	case t == cty.Bool:
		return &PropertySpec{Type: "boolean"}, nil

	case t == cty.DynamicPseudoType:
		return &PropertySpec{Type: "object"}, nil

	case t.IsListType():
		elemSpec, err := ctyTypeToPropertySpec(t.ElementType())
		if err != nil {
			return nil, err
		}
		return &PropertySpec{
			Type:  "array",
			Items: elemSpec,
		}, nil

	case t.IsSetType():
		// Sets are represented as arrays in JSON Schema
		elemSpec, err := ctyTypeToPropertySpec(t.ElementType())
		if err != nil {
			return nil, err
		}
		return &PropertySpec{
			Type:  "array",
			Items: elemSpec,
		}, nil

	case t.IsMapType():
		elemSpec, err := ctyTypeToPropertySpec(t.ElementType())
		if err != nil {
			return nil, err
		}
		return &PropertySpec{
			Type:                 "object",
			AdditionalProperties: elemSpec,
		}, nil

	case t.IsTupleType():
		// Tuples are represented as arrays with the first element type. An empty
		// tuple (e.g. the literal `[]`) has no element type; treat it as an array
		// of any, since the Pulumi schema requires an items type on every array.
		elemTypes := t.TupleElementTypes()
		elem := cty.DynamicPseudoType
		if len(elemTypes) > 0 {
			elem = elemTypes[0]
		}
		elemSpec, err := ctyTypeToPropertySpec(elem)
		if err != nil {
			return nil, err
		}
		return &PropertySpec{
			Type:  "array",
			Items: elemSpec,
		}, nil

	case t.IsObjectType():
		props := make(map[string]*PropertySpec)
		for name, attrType := range t.AttributeTypes() {
			propSpec, err := ctyTypeToPropertySpec(attrType)
			if err != nil {
				return nil, err
			}
			props[name] = propSpec
		}
		return &PropertySpec{
			Type:       "object",
			Properties: props,
		}, nil

	default:
		// Fall back to object type for unknown types
		return &PropertySpec{Type: "object"}, nil
	}
}

// ToJSON serializes the schema to JSON.
func (s *ModuleSchema) ToJSON() ([]byte, error) {
	return json.MarshalIndent(s, "", "  ")
}

// ToPulumiPackageSchema converts the module schema to a full Pulumi package schema format.
func (s *ModuleSchema) ToPulumiPackageSchema() map[string]any {
	componentToken := fmt.Sprintf("%s:%s:%s", s.PackageName, s.Module, s.ComponentName)

	// Build input properties
	inputProps := make(map[string]any)
	for name, prop := range s.InputProperties {
		inputProps[name] = propertySpecToSchemaProperty(prop)
	}

	// Build output properties (inputs + outputs)
	outputProps := make(map[string]any)
	for name, prop := range s.InputProperties {
		outputProps[name] = propertySpecToSchemaProperty(prop)
	}
	for name, prop := range s.OutputProperties {
		outputProps[name] = propertySpecToSchemaProperty(prop)
	}

	return map[string]any{
		"name":        s.PackageName,
		"version":     s.Version,
		"description": s.Description,
		"resources": map[string]any{
			componentToken: map[string]any{
				"isComponent":     true,
				"description":     s.Description,
				"inputProperties": inputProps,
				"requiredInputs":  s.RequiredInputs,
				"properties":      outputProps,
				"type":            "object",
			},
		},
	}
}

// propertySpecToSchemaProperty converts a PropertySpec to a Pulumi schema property format.
func propertySpecToSchemaProperty(prop *PropertySpec) map[string]any {
	result := make(map[string]any)

	if prop.Type != "" {
		result["type"] = prop.Type
	}
	if prop.Description != "" {
		result["description"] = prop.Description
	}
	if prop.Secret {
		result["secret"] = true
	}
	if prop.Default != nil {
		result["default"] = prop.Default
	}
	if prop.Items != nil {
		result["items"] = propertySpecToSchemaProperty(prop.Items)
	}
	if prop.AdditionalProperties != nil {
		result["additionalProperties"] = propertySpecToSchemaProperty(prop.AdditionalProperties)
	}
	if len(prop.Properties) > 0 {
		props := make(map[string]any)
		for name, p := range prop.Properties {
			props[name] = propertySpecToSchemaProperty(p)
		}
		result["properties"] = props
	}
	if prop.Ref != "" {
		result["$ref"] = prop.Ref
	}

	return result
}
