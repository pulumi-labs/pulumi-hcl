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

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/zclconf/go-cty/cty"
)

// ModuleSchema represents a generated schema for an HCL module.
type ModuleSchema struct {
	// Name is the module name.
	Name string `json:"name"`

	// Version is the module version.
	Version string `json:"version,omitempty"`

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
func GenerateModuleSchema(config *ast.Config, moduleName, version string) (*ModuleSchema, error) {
	schema := &ModuleSchema{
		Name:             moduleName,
		Version:          version,
		InputProperties:  make(map[string]*PropertySpec),
		OutputProperties: make(map[string]*PropertySpec),
	}

	// Process variables as inputs
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

	// Process outputs
	for _, o := range config.Outputs {
		prop := outputToPropertySpec(o)
		schema.OutputProperties[o.Name] = prop
	}

	return schema, nil
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

	return prop, nil
}

// outputToPropertySpec converts an HCL output to a PropertySpec.
func outputToPropertySpec(o *ast.Output) *PropertySpec {
	return &PropertySpec{
		Description: o.Description,
		Secret:      o.Sensitive,
		// Outputs don't have explicit types in HCL, so we use "object" (any)
		Type: "object",
	}
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
		// Tuples are represented as arrays with the first element type
		elemTypes := t.TupleElementTypes()
		if len(elemTypes) == 0 {
			return &PropertySpec{Type: "array"}, nil
		}
		elemSpec, err := ctyTypeToPropertySpec(elemTypes[0])
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
// This is useful for generating SDKs or publishing to the Pulumi Registry.
func (s *ModuleSchema) ToPulumiPackageSchema(namespace string) map[string]any {
	componentToken := fmt.Sprintf("%s:modules:%s", namespace, s.Name)

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
		"name":        s.Name,
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
