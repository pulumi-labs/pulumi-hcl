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
	"context"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strings"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	pulumischema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/zclconf/go-cty/cty"
)

// ResourceTypeResolver resolves a TF resource or data source type to its Pulumi
// schema and bridge mapping, so an output expression that references a resource
// or data source can be typed. It is satisfied by *packages.Resolver.
type ResourceTypeResolver interface {
	ResolveResource(ctx context.Context, tfType string) (*pulumischema.Resource, error)
	ResolveFunction(ctx context.Context, tfType string) (*pulumischema.Function, error)
	ResourceBodyMapping(ctx context.Context, tfType string) *bridge.BodyMapping
	DataSourceBodyMapping(ctx context.Context, tfType string) *bridge.BodyMapping
}

// ModuleLoader loads a child module's parsed configuration and resolved
// directory, so module.<name>.<output> references can be typed by recursively
// typing the child module's outputs.
type ModuleLoader interface {
	LoadModule(source, versionConstraint, callerDir string) (config *ast.Config, dir string, err error)
}

// Binder supplies the external lookups used to type output expressions that
// reference resources, data sources, or child modules. A nil *Binder, or a nil
// field within it, leaves the corresponding references untyped (they fall back
// to "any"). Variables and locals are always typed, without a Binder.
type Binder struct {
	// Resources types resource and data source references from provider schemas.
	Resources ResourceTypeResolver
	// Modules loads child modules so their outputs can be typed.
	Modules ModuleLoader
	// ModuleDir is the directory child module sources are resolved relative to.
	ModuleDir string
}

func (b *Binder) child(dir string) *Binder {
	return &Binder{Resources: b.Resources, Modules: b.Modules, ModuleDir: dir}
}

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

// GenerateModuleSchema generates a Pulumi schema from an HCL module
// configuration. binder types output expressions that reference resources, data
// sources, or child modules; pass nil to leave those references untyped.
func GenerateModuleSchema(
	ctx context.Context, config *ast.Config, binder *Binder,
	pkgName, pkgVersion, componentName, module string,
) (*ModuleSchema, error) {
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

	scope, err := buildTypeScope(ctx, config, binder, map[string]bool{})
	if err != nil {
		return nil, err
	}
	evaluator := eval.NewEvaluator(scope)
	for _, o := range config.Outputs {
		ty, err := inferOutputType(evaluator, o)
		if err != nil {
			return nil, fmt.Errorf("typing output %q: %w", o.Name, err)
		}
		prop, err := outputToPropertySpec(o, ty)
		if err != nil {
			return nil, fmt.Errorf("processing output %q: %w", o.Name, err)
		}
		schema.OutputProperties[o.Name] = prop
	}

	sort.Strings(schema.RequiredInputs)

	return schema, nil
}

// buildTypeScope returns an evaluation context in which variable, local,
// resource, data source, and module references resolve to unknown values of
// their types, so that an output expression can be evaluated for its type alone.
// Type information rides through cty evaluation, so typing an output is just
// evaluating it against typed unknowns and reading the result type.
//
// Every reference must resolve: a resource/data source/module that cannot be
// resolved, or a local that cannot be typed (an unresolvable reference or a
// cycle), is a hard error rather than a silent fallback to "any". path holds the
// module directories on the current recursion stack to detect module cycles.
func buildTypeScope(
	ctx context.Context, config *ast.Config, binder *Binder, path map[string]bool,
) (*eval.Context, error) {
	scope := eval.NewContext(".", ".", ".", "", "", "")

	for name, v := range config.Variables {
		t := v.TypeConstraint
		if t == cty.NilType {
			t = cty.DynamicPseudoType
		}
		scope.SetVariable(name, cty.UnknownVal(t))
	}

	if binder != nil {
		if err := seedResourceTypes(ctx, scope, config, binder.Resources); err != nil {
			return nil, err
		}
		if err := seedModuleTypes(ctx, scope, config, binder, path); err != nil {
			return nil, err
		}
	}

	if err := seedLocalTypes(scope, config); err != nil {
		return nil, err
	}

	return scope, nil
}

// seedLocalTypes types each local against the (already seeded) scope and binds
// it. Locals may reference variables, resources, modules, and one another, so it
// evaluates to a fixpoint: each pass types any local whose references already
// resolve. If a pass makes no progress while locals remain, those locals cannot
// be typed (an unresolvable reference or a local cycle) and the first such
// failure is returned.
func seedLocalTypes(scope *eval.Context, config *ast.Config) error {
	evaluator := eval.NewEvaluator(scope)
	remaining := maps.Clone(config.Locals)
	for len(remaining) > 0 {
		progress := false
		for name, l := range remaining {
			val, diags := evaluator.Evaluate(l.Value)
			if diags.HasErrors() {
				continue
			}
			scope.SetLocal(name, cty.UnknownVal(val.Type()))
			delete(remaining, name)
			progress = true
		}
		if !progress {
			for _, name := range slices.Sorted(maps.Keys(remaining)) {
				_, diags := evaluator.Evaluate(remaining[name].Value)
				return fmt.Errorf("typing local %q: %s", name, diags.Error())
			}
		}
	}
	return nil
}

// rangedType wraps an element reference type to match how a count or for_each
// reference is shaped: count yields a list of instances, for_each a map of them.
func rangedType(elem cty.Type, count, forEach bool) cty.Type {
	switch {
	case count:
		return cty.List(elem)
	case forEach:
		return cty.Map(elem)
	default:
		return elem
	}
}

// seedResourceTypes binds each resource and data source to an unknown value of
// its reference type, resolved from its provider schema. Ranged (count/for_each)
// references are bound as a list/map of the element type. A type that cannot be
// resolved is an error.
func seedResourceTypes(
	ctx context.Context, scope *eval.Context, config *ast.Config, resolver ResourceTypeResolver,
) error {
	if resolver == nil {
		if len(config.Resources) > 0 || len(config.DataSources) > 0 {
			return fmt.Errorf("cannot type resource references: no resource resolver provided")
		}
		return nil
	}
	for _, key := range slices.Sorted(maps.Keys(config.Resources)) {
		res := config.Resources[key]
		schemaRes, err := resolver.ResolveResource(ctx, res.Type)
		if err != nil {
			return fmt.Errorf("resolving resource %q: %w", key, err)
		}
		if schemaRes == nil {
			return fmt.Errorf("resolving resource %q: no schema for type %q", key, res.Type)
		}
		mapping := resolver.ResourceBodyMapping(ctx, res.Type)
		ty := rangedType(transform.ResourceReferenceType(schemaRes, mapping), res.Count != nil, res.ForEach != nil)
		scope.SetResource(key, urn.URN(""), cty.UnknownVal(ty))
	}

	for _, key := range slices.Sorted(maps.Keys(config.DataSources)) {
		ds := config.DataSources[key]
		fn, err := resolver.ResolveFunction(ctx, ds.Type)
		if err != nil {
			return fmt.Errorf("resolving data source %q: %w", key, err)
		}
		if fn == nil {
			return fmt.Errorf("resolving data source %q: no schema for type %q", key, ds.Type)
		}
		mapping := resolver.DataSourceBodyMapping(ctx, ds.Type)
		ty := rangedType(transform.DataSourceReferenceType(fn, mapping), ds.Count != nil, ds.ForEach != nil)
		scope.SetDataSource(key, cty.UnknownVal(ty))
	}
	return nil
}

// seedModuleTypes binds each module call to an unknown value of its output
// object type, computed by recursively typing the child module's outputs.
// Ranged calls are bound as a list/map of that object. A module that cannot be
// loaded, or whose source is already on the recursion path (a module cycle), is
// an error.
func seedModuleTypes(
	ctx context.Context, scope *eval.Context, config *ast.Config, binder *Binder, path map[string]bool,
) error {
	if binder.Modules == nil {
		if len(config.Modules) > 0 {
			return fmt.Errorf("cannot type module references: no module loader provided")
		}
		return nil
	}
	for _, name := range slices.Sorted(maps.Keys(config.Modules)) {
		call := config.Modules[name]
		childConfig, dir, err := binder.Modules.LoadModule(call.Source, call.Version, binder.ModuleDir)
		if err != nil {
			return fmt.Errorf("loading module %q: %w", name, err)
		}
		if path[dir] {
			return fmt.Errorf("module cycle through %q (%s)", name, dir)
		}

		path[dir] = true
		childScope, err := buildTypeScope(ctx, childConfig, binder.child(dir), path)
		delete(path, dir)
		if err != nil {
			return fmt.Errorf("typing module %q: %w", name, err)
		}

		childEval := eval.NewEvaluator(childScope)
		attrs := make(map[string]cty.Type, len(childConfig.Outputs))
		for _, o := range childConfig.Outputs {
			ty, err := inferOutputType(childEval, o)
			if err != nil {
				return fmt.Errorf("typing module %q output %q: %w", name, o.Name, err)
			}
			attrs[o.Name] = ty
		}
		ty := rangedType(cty.Object(attrs), call.Count != nil, call.ForEach != nil)
		scope.SetModule(name, cty.UnknownVal(ty))
	}
	return nil
}

// inferOutputType evaluates an output's value expression against the type scope
// and returns its type. An output with no value, or whose expression cannot be
// evaluated against the scope, is an error.
func inferOutputType(evaluator *eval.Evaluator, o *ast.Output) (cty.Type, error) {
	if o.Value == nil {
		return cty.NilType, fmt.Errorf("output has no value expression")
	}
	val, diags := evaluator.Evaluate(o.Value)
	if diags.HasErrors() {
		return cty.NilType, fmt.Errorf("%s", diags.Error())
	}
	return val.Type(), nil
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

// pulumiName converts a snake_case HCL identifier (a variable or output name) to
// the camelCase name Pulumi conventionally uses for the corresponding schema
// property. The runtime boundary (Construct) reverses this with
// transform.SnakeCaseFromPulumiCase to recover the HCL name.
func pulumiName(snake string) string {
	name, _ := transform.PulumiCaseFromSnakeCase(snake, nil)
	return name
}

// InputsToHCL maps component inputs — keyed by their camelCase schema property
// names — to the snake_case names the HCL module declares, recursively renaming
// object fields at every depth. Map keys are user data and are left unchanged.
func (s *ModuleSchema) InputsToHCL(inputs resource.PropertyMap) resource.PropertyMap {
	out := make(resource.PropertyMap, len(inputs))
	for k, v := range inputs {
		snake := transform.SnakeCaseFromPulumiCase(string(k))
		out[resource.PropertyKey(snake)] = convertPropertyNames(v, s.InputProperties[snake], false)
	}
	return out
}

// OutputsToPulumi maps the module's outputs — keyed by their snake_case HCL
// names — to the camelCase schema property names, recursively renaming object
// fields at every depth. Map keys are left unchanged.
func (s *ModuleSchema) OutputsToPulumi(outputs resource.PropertyMap) resource.PropertyMap {
	out := make(resource.PropertyMap, len(outputs))
	for k, v := range outputs {
		out[resource.PropertyKey(pulumiName(string(k)))] = convertPropertyNames(v, s.OutputProperties[string(k)], true)
	}
	return out
}

// convertPropertyNames recursively renames object-field names in v according to
// spec, which describes v's type. Object fields (spec.Properties, snake_case)
// are renamed between snake_case and camelCase; the dynamic keys of a map
// (spec.AdditionalProperties) are user data and left unchanged. toPulumi selects
// the direction: snake_case→camelCase when true, the reverse when false.
func convertPropertyNames(v resource.PropertyValue, spec *PropertySpec, toPulumi bool) resource.PropertyValue {
	switch {
	case spec == nil:
		return v
	case v.IsSecret():
		return resource.MakeSecret(convertPropertyNames(v.SecretValue().Element, spec, toPulumi))
	case v.IsOutput():
		o := v.OutputValue()
		o.Element = convertPropertyNames(o.Element, spec, toPulumi)
		return resource.NewOutputProperty(o)
	case len(spec.Properties) > 0 && v.IsObject():
		obj := v.ObjectValue()
		out := make(resource.PropertyMap, len(obj))
		for snake, fieldSpec := range spec.Properties {
			src, dst := resource.PropertyKey(pulumiName(snake)), resource.PropertyKey(snake)
			if toPulumi {
				src, dst = dst, src
			}
			if fv, ok := obj[src]; ok {
				out[dst] = convertPropertyNames(fv, fieldSpec, toPulumi)
			}
		}
		return resource.NewObjectProperty(out)
	case spec.AdditionalProperties != nil && v.IsObject():
		obj := v.ObjectValue()
		out := make(resource.PropertyMap, len(obj))
		for k, mv := range obj {
			out[k] = convertPropertyNames(mv, spec.AdditionalProperties, toPulumi)
		}
		return resource.NewObjectProperty(out)
	case spec.Items != nil && v.IsArray():
		arr := v.ArrayValue()
		out := make([]resource.PropertyValue, len(arr))
		for i, e := range arr {
			out[i] = convertPropertyNames(e, spec.Items, toPulumi)
		}
		return resource.NewArrayProperty(out)
	default:
		return v
	}
}

// ToPulumiPackageSchema converts the module schema to a full Pulumi package
// schema format. Variable and output names, which are snake_case in HCL, are
// exposed under their camelCase Pulumi property names. Object types are emitted
// as named types in the `types` section and referenced via `$ref`, so a consumer
// binds them as proper object types (a property's inline type spec cannot carry
// `properties`).
func (s *ModuleSchema) ToPulumiPackageSchema() map[string]any {
	componentToken := fmt.Sprintf("%s:%s:%s", s.PackageName, s.Module, s.ComponentName)
	types := make(map[string]any)

	inputProps := make(map[string]any)
	for name, prop := range s.InputProperties {
		inputProps[pulumiName(name)] = s.schemaProperty(prop, pascalCase(pulumiName(name)), types)
	}

	// Output properties are the inputs plus the declared outputs.
	outputProps := make(map[string]any)
	for name, prop := range s.InputProperties {
		outputProps[pulumiName(name)] = s.schemaProperty(prop, pascalCase(pulumiName(name)), types)
	}
	for name, prop := range s.OutputProperties {
		outputProps[pulumiName(name)] = s.schemaProperty(prop, pascalCase(pulumiName(name)), types)
	}

	var requiredInputs []string
	for _, name := range s.RequiredInputs {
		requiredInputs = append(requiredInputs, pulumiName(name))
	}
	sort.Strings(requiredInputs)

	result := map[string]any{
		"name":        s.PackageName,
		"version":     s.Version,
		"description": s.Description,
		"resources": map[string]any{
			componentToken: map[string]any{
				"isComponent":     true,
				"description":     s.Description,
				"inputProperties": inputProps,
				"requiredInputs":  requiredInputs,
				"properties":      outputProps,
				"type":            "object",
			},
		},
	}
	if len(types) > 0 {
		result["types"] = types
	}
	return result
}

// pascalCase upper-cases the first letter of a camelCase name, for use in a type
// token.
func pascalCase(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// schemaProperty converts a PropertySpec to a Pulumi property type spec. An
// object type is registered as a named type (keyed by a token derived from
// typeName) in types and referenced via `$ref`; the dynamic keys of a map are
// not named, so its value type recurses without registering field names.
func (s *ModuleSchema) schemaProperty(prop *PropertySpec, typeName string, types map[string]any) map[string]any {
	result := make(map[string]any)

	switch {
	case len(prop.Properties) > 0:
		token := fmt.Sprintf("%s:%s:%s", s.PackageName, s.Module, typeName)
		fields := make(map[string]any)
		for name, field := range prop.Properties {
			camel := pulumiName(name)
			fields[camel] = s.schemaProperty(field, typeName+pascalCase(camel), types)
		}
		types[token] = map[string]any{"type": "object", "properties": fields}
		result["$ref"] = "#/types/" + token
	case prop.AdditionalProperties != nil:
		result["type"] = "object"
		result["additionalProperties"] = s.schemaProperty(prop.AdditionalProperties, typeName+"Value", types)
	case prop.Items != nil:
		result["type"] = "array"
		result["items"] = s.schemaProperty(prop.Items, typeName+"Item", types)
	case prop.Type != "":
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
	return result
}
