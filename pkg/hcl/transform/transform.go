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

// Package transform handles conversion between cty values and Pulumi property values.
package transform

import (
	"fmt"
	"os"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/cgstrings"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/archive"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/asset"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
)

const SensativeMark = "sensitive"

var resourceRefCapsuleType = cty.Capsule("resource_reference", reflect.TypeOf(property.ResourceReference{}))

type EvalFunc = func(resource.PropertyKey, hcl.Expression) (cty.Value, hcl.Diagnostics)

func EvalFunctionWithSchema(config hcl.Body, r *schema.Function, eval EvalFunc) (property.Map, hcl.Diagnostics) {
	var props []*schema.Property
	if r.Inputs != nil {
		props = r.Inputs.Properties
	}
	functionInputs, diags := evalBlockWithSchema(config, props, eval)
	if diags.HasErrors() {
		return property.Map{}, diags
	}

	m, err := ctyToFunctionInputs(functionInputs, r)
	if err != nil {
		return property.Map{}, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "failed to convert HCL function inputs to Pulumi inputs",
			Detail:   err.Error(),
		})
	}
	return m, diags
}

func EvalResourceWithSchema(config hcl.Body, r *schema.Resource, eval EvalFunc) (property.Map, hcl.Diagnostics) {
	resourceInputs, diags := evalBlockWithSchema(config, r.InputProperties, eval)
	if diags.HasErrors() {
		return property.Map{}, diags
	}

	m, err := ctyToResourceInputs(resourceInputs, r)
	if err != nil {
		return property.Map{}, append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "failed to convert HCL resource inputs to Pulumi inputs",
			Detail:   err.Error(),
		})
	}
	return m, diags
}

func evalBlocksWithSchema(config hcl.Blocks, props []*schema.Property, eval EvalFunc) ([]cty.Value, hcl.Diagnostics) {
	out := make([]cty.Value, len(config))
	var diags hcl.Diagnostics
	for i, v := range config {
		evaluated, diag := evalBlockWithSchema(v.Body, props, eval)
		diags = diags.Extend(diag)
		if diag.HasErrors() {
			return nil, diags
		}
		out[i] = evaluated
	}
	return out, diags
}

func evalBlockWithSchema(config hcl.Body, props []*schema.Property, eval EvalFunc) (cty.Value, hcl.Diagnostics) {
	body, diags := config.Content(inputBodyFromProperties(props))
	if diags.HasErrors() {
		return cty.Value{}, diags
	}

	if len(props) == 0 {
		return cty.EmptyObjectVal, diags
	}

	resourceInputs := make(map[string]cty.Value, len(body.Attributes)+len(body.Blocks))
	for name, attr := range body.Attributes {
		_, prop := camelCaseFromSnakeCase(name, props)
		contract.Assertf(prop != nil, "unable to find schema for validated property")

		out, attrDiag := eval(resource.PropertyKey(prop.Name), attr.Expr)
		diags = diags.Extend(attrDiag)
		if attrDiag.HasErrors() {
			return cty.Value{}, diags
		}
		resourceInputs[name] = conformCtyToType(out, ctyTypeFromType(prop.Type))
	}

	for name, blocks := range body.Blocks.ByType() {
		var prop *schema.Property
		for _, p := range props {
			if snakeCaseFromCamelCase(p.Name) == name {
				prop = p
				break
			}
		}
		contract.Assertf(prop != nil, "unable to find schema for validated property")

		blockType, _ := AsHCLBlockType(prop.Type)
		values, diags := evalBlocksWithSchema(blocks, blockType.Properties, eval)
		if diags.HasErrors() {
			return cty.Value{}, diags
		}
		resourceInputs[name] = cty.ListVal(values)
	}

	return cty.ObjectVal(resourceInputs), diags
}

func conformCtyToType(val cty.Value, typ cty.Type) cty.Value {
	if val.Type().Equals(typ) {
		return val
	}

	if val.Type().IsObjectType() && typ.IsMapType() {
		m := make(map[string]cty.Value, val.LengthInt())
		for attrs := val.ElementIterator(); attrs.Next(); {
			k, v := attrs.Element()
			m[k.AsString()] = conformCtyToType(v, typ.ElementType())
		}
		if len(m) == 0 {
			return cty.MapValEmpty(typ.ElementType())
		}
		return cty.MapVal(m)
	}

	return val
}

// AsHCLBlockType reports whether typ is a List<Object> type that should be
// rendered as repeated HCL blocks rather than an attribute. If so it returns
// the inner ObjectType.
func AsHCLBlockType(typ schema.Type) (*schema.ObjectType, bool) {
	arr, ok := codegen.UnwrapType(typ).(*schema.ArrayType)
	if !ok {
		return nil, false
	}
	obj, ok := codegen.UnwrapType(arr.ElementType).(*schema.ObjectType)
	return obj, ok
}

func inputBodyFromProperties(r []*schema.Property) *hcl.BodySchema {
	body := new(hcl.BodySchema)
	for _, p := range r {
		typeName := snakeCaseFromCamelCase(p.Name)
		if _, ok := AsHCLBlockType(p.Type); ok {
			body.Blocks = append(body.Blocks, hcl.BlockHeaderSchema{
				Type: typeName,
			})
			continue
		}
		body.Attributes = append(body.Attributes, hcl.AttributeSchema{
			Name:     typeName,
			Required: p.IsRequired(),
		})
	}
	return body
}

func ctyToResourceInputs(val cty.Value, r *schema.Resource) (property.Map, error) {
	return ctyToObject(r.Token, val, r.InputProperties, false /* already in a secret */)
}

func ctyToFunctionInputs(val cty.Value, r *schema.Function) (property.Map, error) {
	var inputs []*schema.Property
	if r.Inputs != nil {
		inputs = r.Inputs.Properties
	}
	return ctyToObject(r.Token, val, inputs, false /* already in a secret */)
}

func ctyToObject(path string, val cty.Value, properties []*schema.Property, alreadyInSecret bool) (property.Map, error) {
	seen := make(map[string]struct{})
	result := map[string]property.Value{}
	for it := val.ElementIterator(); it.Next(); {
		k, v := it.Element()
		puField, prop := camelCaseFromSnakeCase(k.AsString(), properties)
		if prop == nil {
			// We have not found the correct field in the property list, so we should put together an error
			// message.
			paths := make([]string, len(properties))
			for i, p := range properties {
				paths[i] = p.Name
			}
			return property.Map{}, fmt.Errorf("could not find %q (translated from %q) in %q: %s",
				puField, k.AsString(), path, strings.Join(paths, ", "))
		}
		seen[puField] = struct{}{}
		var err error
		result[puField], err = ctyToResourceProperty(k.AsString(), v, prop.Type, prop.Secret || alreadyInSecret)
		if err != nil {
			return property.Map{}, err
		}
		if prop.Secret && !alreadyInSecret {
			result[puField] = result[puField].WithSecret(true)
		}
	}

	for _, prop := range properties {
		_, ok := seen[prop.Name]
		if ok {
			continue
		}
		if d := prop.DefaultValue; d != nil {
			v, err := getDefault(snakeCaseFromCamelCase(prop.Name), d, prop.Type)
			if err != nil {
				return property.Map{}, err
			}
			result[prop.Name] = v.WithSecret(prop.Secret)
			if prop.Secret {
				result[prop.Name] = result[prop.Name].WithSecret(true)
			}
		}
	}
	return property.NewMap(result), nil
}

func getDefault(path string, d *schema.DefaultValue, typ schema.Type) (property.Value, error) {
	for _, env := range d.Environment {
		if v := os.Getenv(env); v != "" {
			switch typ {
			case schema.BoolType:
				return property.New(cmdutil.IsTruthy(v)), nil
			case schema.NumberType:
				n, err := strconv.ParseFloat(v, 64)
				if err != nil {
					return property.Value{}, fmt.Errorf("%q: unable to parse %q into a number: %w",
						path, v, err)
				}
				return property.New(n), nil
			default:
				return property.New(v), nil
			}
		}
	}
	if v := d.Value; v != nil {
		if i, ok := d.Value.(int); ok {
			v = float64(i)
		}
		v, err := property.Any(v)
		if err != nil {
			// This indicates an invalid schema
			return property.Value{}, fmt.Errorf("%q: reading default value from %#v: %w",
				path, v, err)
		}
		return v, nil
	}
	return property.New(property.Null), nil
}

func ctyToResourceProperty(path string, val cty.Value, prop schema.Type, alreadyInSecret bool) (property.Value, error) {
	if val.IsMarked() {
		var marks cty.ValueMarks
		val, marks = val.Unmark()
		if _, isSensitive := marks[SensativeMark]; isSensitive && !alreadyInSecret {
			v, err := ctyToResourceProperty(path, val, prop, true)
			return v.WithSecret(true), err
		}
	}

	// Strip unneeded signifiers
	prop = codegen.UnwrapType(prop)

	// Handle primitive types & unknown

	switch {
	case !val.IsKnown():
		return property.New(property.Computed), nil
	case val.Type().Equals(cty.String):
		return property.New(val.AsString()), nil
	case val.Type().Equals(cty.Bool):
		return property.New(val.True()), nil
	case val.Type().Equals(cty.Number):
		f, _ := val.AsBigFloat().Float64()
		return property.New(f), nil
	}

	// We don't have any type info, so do a direct conversion.
	if prop == schema.AnyType {
		return ctyToPropertyValue(val)
	}

	// Handle asset and archive types.
	if prop == schema.AssetType || prop == schema.ArchiveType {
		if !val.Type().IsCapsuleType() {
			return property.Value{}, fmt.Errorf("%q: expected asset or archive, got %s", path, val.Type().FriendlyName())
		}
		switch inner := val.EncapsulatedValue().(type) {
		case *asset.Asset:
			return property.New(inner), nil
		case *archive.Archive:
			return property.New(inner), nil
		default:
			return property.Value{}, fmt.Errorf("%q: unexpected capsule value type %T", path, inner)
		}
	}

	// Handle complex types
	switch prop := prop.(type) {
	case *schema.ResourceType:
		if !val.Type().IsObjectType() {
			return property.Value{}, fmt.Errorf("expected object at %q for resource reference, found %#v", path, val.Type())
		}
		refAttr, hasRef := val.AsValueMap()["__ref"]
		if !hasRef || !refAttr.IsKnown() {
			return property.New(property.Computed), nil
		}
		ref, ok := refAttr.EncapsulatedValue().(*property.ResourceReference)
		if !ok {
			return property.Value{}, fmt.Errorf("expected resource reference capsule at %q", path)
		}
		return property.New(*ref), nil
	case *schema.ObjectType:
		if !val.Type().IsObjectType() {
			return property.Value{}, fmt.Errorf("expected object at %q, found %#v", path, val.Type())
		}
		m, err := ctyToObject(path, val, prop.Properties, alreadyInSecret)
		return property.New(m), err
	case *schema.ArrayType:
		if !val.Type().IsListType() && !val.Type().IsSetType() && !val.Type().IsTupleType() {
			return property.Value{}, fmt.Errorf("expected list or set at %q, found %#v", path, val.Type())
		}
		arr := make([]property.Value, 0, val.LengthInt())
		for it := val.ElementIterator(); it.Next(); {
			_, elem := it.Element()
			convertedElem, err := ctyToResourceProperty(fmt.Sprintf("%s[%d]", path, len(arr)), elem, prop.ElementType, alreadyInSecret)
			if err != nil {
				return property.Value{}, err
			}
			arr = append(arr, convertedElem)
		}
		return property.New(arr), nil
	case *schema.MapType:
		if !val.Type().IsMapType() && !val.Type().IsObjectType() {
			return property.Value{}, fmt.Errorf("expected map at %q, found %#v", path, val.Type())
		}
		m := make(map[string]property.Value, val.LengthInt())
		for it := val.ElementIterator(); it.Next(); {
			k, elem := it.Element()
			convertedElem, err := ctyToResourceProperty(fmt.Sprintf("%s[%q]", path, k.AsString()), elem, prop.ElementType, alreadyInSecret)
			if err != nil {
				return property.Value{}, err
			}
			m[k.AsString()] = convertedElem
		}
		return property.New(m), nil
	case *schema.UnionType:
		return ctyToPropertyValue(val)
	default:
		return property.Value{}, fmt.Errorf("%q: unknown schema type %s when converting %#v", path, prop, val.Type())
	}
}

func camelCaseFromSnakeCase(s string, props []*schema.Property) (string, *schema.Property) {
	for _, p := range props {
		if snakeCaseFromCamelCase(p.Name) == s {
			return p.Name, p
		}
	}
	return cgstrings.ModifyStringAroundDelimeter(s, "_", cgstrings.UppercaseFirst), nil
}

func SnakeCaseFromPulumiCase(s string) string {
	return snakeCaseFromCamelCase(s)
}

// Convert from camelCase to snake_case.
//
// This function looses information, since we cannot distinguish between SCREAM & scream.
func snakeCaseFromCamelCase(s string) string {
	var b strings.Builder
	runes := []rune(s)
	var fromCapital bool

	for i, c := range runes {
		switch {
		case unicode.IsUpper(c):
			if b.Len() == 0 {
				b.WriteRune(unicode.ToLower(c))
				fromCapital = true
				continue
			}

			// If the previous letter was capitalized, and the next letter is capitalized, continue the
			// word.
			if fromCapital && (len(runes) <= i+1 || unicode.IsUpper(runes[i+1]) || unicode.IsNumber(runes[i+1])) {
				b.WriteRune(unicode.ToLower(c))
				continue
			}

			// Otherwise start a new word
			b.WriteRune('_')
			b.WriteRune(unicode.ToLower(c))
			fromCapital = true
		default:
			fromCapital = false
			b.WriteRune(c)
		}
	}
	return b.String()
}

// CtyToPropertyValue converts a cty.Value to a Pulumi PropertyValue.
//
// Because this conversion is untyped, it should be avoided when type information is available.
func CtyToPropertyValue(val cty.Value) (property.Value, error) {
	return ctyToPropertyValue(val)
}

func ctyToPropertyValue(val cty.Value) (property.Value, error) {
	// Handle sensitive-marked values by unwrapping, converting, and wrapping as secret.
	if val.IsMarked() {
		unmarked, marks := val.Unmark()
		_, isSensitive := marks[SensativeMark]
		pv, err := ctyToPropertyValue(unmarked)
		return pv.WithSecret(isSensitive), err
	}

	if val.IsNull() {
		return property.New(property.Null), nil
	}

	if !val.IsKnown() {
		// Unknown values are represented as computed in Pulumi
		return property.New(property.Computed), nil
	}

	typ := val.Type()

	switch {
	case typ == cty.Bool:
		return property.New(val.True()), nil

	case typ == cty.String:
		return property.New(val.AsString()), nil

	case typ == cty.Number:
		f64, _ := val.AsBigFloat().Float64()
		return property.New(f64), nil

	case typ.IsListType() || typ.IsTupleType():
		return ctyListToPropertyValue(val)

	case typ.IsSetType():
		return ctySetToPropertyValue(val)

	case typ.IsMapType() || typ.IsObjectType():
		return ctyObjectToPropertyValue(val)

	case typ == cty.DynamicPseudoType:
		// Dynamic type - try to infer the type from the underlying value
		return property.New(property.Null), nil

	case typ.IsCapsuleType():
		switch inner := val.EncapsulatedValue().(type) {
		case *asset.Asset:
			return property.New(inner), nil
		case *archive.Archive:
			return property.New(inner), nil
		default:
			return property.Value{}, fmt.Errorf("unknown capsule type %T", inner)
		}

	default:
		return property.Value{}, fmt.Errorf("unknown type %v", typ)
	}
}

// ctyListToPropertyValue converts a cty list/tuple to a Pulumi array.
func ctyListToPropertyValue(val cty.Value) (property.Value, error) {
	arr := make([]property.Value, 0, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		_, elemVal := it.Element()
		pv, err := ctyToPropertyValue(elemVal)
		if err != nil {
			return property.Value{}, err
		}
		arr = append(arr, pv)
	}
	return property.New(arr), nil
}

// ctySetToPropertyValue converts a cty set to a Pulumi array.
func ctySetToPropertyValue(val cty.Value) (property.Value, error) {
	var arr []property.Value
	for it := val.ElementIterator(); it.Next(); {
		_, elemVal := it.Element()
		pv, err := ctyToPropertyValue(elemVal)
		if err != nil {
			return property.Value{}, err
		}
		arr = append(arr, pv)
	}
	return property.New(arr), nil
}

// ctyObjectToPropertyValue converts a cty map/object to a Pulumi object.
func ctyObjectToPropertyValue(val cty.Value) (property.Value, error) {
	obj := make(map[string]property.Value, val.LengthInt())
	for it := val.ElementIterator(); it.Next(); {
		keyVal, elemVal := it.Element()
		key := keyVal.AsString()
		pv, err := ctyToPropertyValue(elemVal)
		if err != nil {
			return property.Value{}, err
		}
		obj[key] = pv
	}
	return property.New(obj), nil
}

func ResourceOutputToCty(pv property.Map, r *schema.Resource, dryRun bool) (map[string]cty.Value, error) {
	properties := r.Properties
	// Providers pass "version" as an output - even though it's not in the schema.
	if r.IsProvider {
		properties = append(slices.Clone(r.Properties), &schema.Property{
			Name: "version",
			Type: schema.StringType,
		})
	}
	return propertyObjectToCtyMap("", pv, properties, dryRun)
}

func FunctionOutputToCty(pv property.Map, r *schema.Function, dryRun bool) (cty.Value, error) {
	if obj, ok := r.ReturnType.(*schema.ObjectType); ok {
		o, err := propertyObjectToCtyMap("", pv, obj.Properties, dryRun)
		return cty.ObjectVal(o), err
	}
	for k, scalarPV := range pv.AsMap() {
		return propertyValueToCty(k, scalarPV, r.ReturnType, dryRun)
	}
	if dryRun {
		return cty.UnknownVal(ctyTypeFromType(r.ReturnType)), nil
	}
	return cty.Value{}, fmt.Errorf("invoke %q: provider returned empty scalar result", r.Token)
}

func propertyObjectToCtyMap(path string, m property.Map, properties []*schema.Property, dryRun bool) (map[string]cty.Value, error) {
	result := make(map[string]cty.Value, m.Len())
	for _, p := range properties {
		hclName := snakeCaseFromCamelCase(p.Name)
		v, ok := m.GetOk(p.Name)
		if !ok {
			result[hclName] = cty.UnknownVal(ctyTypeFromType(p.Type))
			continue
		}
		// During preview, required properties that are null are treated as unknown.
		// This is because resource.Computed{} (the SDK's representation of an unknown value)
		// has a null inner element, which gets serialized to a null proto value by gRPC
		// marshaling, losing the "unknown" signal. Since a required property should never
		// legitimately be null, we safely treat null-during-preview as unknown.
		if dryRun && v.IsNull() && p.IsRequired() {
			result[hclName] = cty.UnknownVal(ctyTypeFromType(p.Type))
			continue
		}
		var vPath string
		if path == "" {
			vPath = hclName
		} else {
			vPath = path + "." + hclName
		}
		convertedV, err := propertyValueToCty(fmt.Sprintf(vPath, path, hclName), v, p.Type, dryRun)
		if err != nil {
			return nil, err
		}
		result[hclName] = convertedV
	}

	return result, nil
}

func ctyTypeFromType(typ schema.Type) cty.Type {
	typ = codegen.UnwrapType(typ)

	switch typ {
	case schema.StringType:
		return cty.String
	case schema.BoolType:
		return cty.Bool
	case schema.NumberType, schema.IntType:
		return cty.Number
	case schema.AnyType:
		return cty.DynamicPseudoType
	case schema.AssetType:
		return eval.AssetCapsuleType
	case schema.ArchiveType:
		return eval.ArchiveCapsuleType
	}

	switch typ := typ.(type) {
	case *schema.ArrayType:
		return cty.List(ctyTypeFromType(typ.ElementType))
	case *schema.MapType:
		return cty.Map(ctyTypeFromType(typ.ElementType))
	case *schema.EnumType:
		return ctyTypeFromType(typ.ElementType)
	case *schema.ObjectType:
		attrs := make(map[string]cty.Type, len(typ.Properties))
		var optional []string
		for _, p := range typ.Properties {
			key := snakeCaseFromCamelCase(p.Name)
			if !p.IsRequired() {
				optional = append(optional, key)
			}
			attrs[key] = ctyTypeFromType(p.Type)
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional)
	case *schema.ResourceType:
		if typ.Resource == nil {
			return cty.DynamicPseudoType
		}
		attrs := map[string]cty.Type{"__ref": resourceRefCapsuleType}
		optional := []string{"__ref"}
		for _, p := range typ.Resource.Properties {
			key := snakeCaseFromCamelCase(p.Name)
			if !p.IsRequired() {
				optional = append(optional, key)
			}
			attrs[key] = ctyTypeFromType(p.Type)
		}
		return cty.ObjectWithOptionalAttrs(attrs, optional)
	case *schema.InvalidType:
		return cty.DynamicPseudoType
	case *schema.UnionType:
		if typ.DefaultType != nil {
			if t := ctyTypeFromType(typ.DefaultType); t != cty.NilType {
				return t
			}
		}
		for _, t := range typ.ElementTypes {
			if t := ctyTypeFromType(t); t != cty.NilType {
				return t
			}
		}
		return cty.NilType
	default:
		return cty.NilType
	}
}

func propertyValueToCty(path string, v property.Value, typ schema.Type, dryRun bool) (cty.Value, error) {
	typ = codegen.UnwrapType(typ)
	if v.Secret() {
		computedV, err := propertyValueToCty(path, v.WithSecret(false), typ, dryRun)
		return computedV.Mark(SensativeMark), err
	}

	switch {

	// Primitive types

	case v.IsComputed():
		return cty.UnknownVal(ctyTypeFromType(typ)), nil
	case v.IsString():
		return cty.StringVal(v.AsString()), nil
	case v.IsBool():
		return cty.BoolVal(v.AsBool()), nil
	case v.IsNumber():
		return cty.NumberFloatVal(v.AsNumber()), nil
	case v.IsNull():
		return cty.NullVal(ctyTypeFromType(typ)), nil

	// Collection types

	case v.IsMap():
		elemType := schema.AnyType
		switch typ := typ.(type) {
		case *schema.ResourceType:
			if typ.Resource == nil {
				break
			}
			result, err := propertyObjectToCtyMap(path, v.AsMap(), typ.Resource.Properties, dryRun)
			if err != nil {
				return cty.Value{}, err
			}
			if refVal, ok := v.AsMap().GetOk("__ref"); ok && refVal.IsResourceReference() {
				ref := refVal.AsResourceReference()
				result["__ref"] = cty.CapsuleVal(resourceRefCapsuleType, &ref)
			}
			return cty.ObjectVal(result), nil
		case *schema.ObjectType:
			m, err := propertyObjectToCtyMap(path, v.AsMap(), typ.Properties, dryRun)
			if err != nil {
				return cty.Value{}, err
			}
			return cty.ObjectVal(m), nil
		case *schema.MapType:
			elemType = typ.ElementType
		}
		m := make(map[string]cty.Value, v.AsMap().Len())
		for k, v := range v.AsMap().All {
			convertedV, err := propertyValueToCty(fmt.Sprintf("%s[%q]", path, k), v, elemType, dryRun)
			if err != nil {
				return cty.Value{}, err
			}
			m[k] = convertedV
		}
		if len(m) == 0 {
			return cty.MapValEmpty(ctyTypeFromType(elemType)), nil
		}

		// If all elements are not the same - then cty requires an object type.
		//
		// This occurs when we have a non-homogeneous block not typed like an object. For example with the
		// pulumi_stack resource:
		//
		//	 resource "pulumi_stash" "myStash" {
		//	   input = {
		//	     "key" = ["value", "s"]
		//	     ""    = false
		//	   }
		//	 }
		var t *cty.Type
		for _, v := range m {
			if t == nil {
				t = new(v.Type())
			}
			if !v.Type().Equals(*t) {
				return cty.ObjectVal(m), nil
			}
		}

		return cty.MapVal(m), nil

	case v.IsArray():
		elemType := schema.AnyType
		if arr, ok := typ.(*schema.ArrayType); ok {
			elemType = arr.ElementType
		}
		arr := make([]cty.Value, v.AsArray().Len())
		for i, v := range v.AsArray().All {
			convertedV, err := propertyValueToCty(fmt.Sprintf("%s[%d]", path, i), v, elemType, dryRun)
			if err != nil {
				return cty.Value{}, err
			}
			arr[i] = convertedV
		}
		if len(arr) == 0 {
			return cty.ListValEmpty(ctyTypeFromType(elemType)), nil
		}
		return cty.ListVal(arr), nil

	case v.IsResourceReference():
		ref := v.AsResourceReference()
		resType, ok := typ.(*schema.ResourceType)
		if !ok || resType.Resource == nil {
			return cty.NullVal(ctyTypeFromType(typ)), nil
		}
		result, err := propertyObjectToCtyMap(path, property.Map{}, resType.Resource.Properties, dryRun)
		if err != nil {
			return cty.Value{}, err
		}
		result["__ref"] = cty.CapsuleVal(resourceRefCapsuleType, &ref)
		return cty.ObjectVal(result), nil

	case v.IsAsset():
		return cty.CapsuleVal(eval.AssetCapsuleType, v.AsAsset()), nil

	case v.IsArchive():
		return cty.CapsuleVal(eval.ArchiveCapsuleType, v.AsArchive()), nil

	default:
		return cty.Value{}, fmt.Errorf("%s: unhandled property %s", path, v.GoString())
	}
}

// PropertyValueToCty converts a Pulumi PropertyValue to a cty.Value.
func PropertyValueToCty(pv property.Value) cty.Value {

	if pv.Secret() {
		return PropertyValueToCty(pv.WithSecret(false)).
			WithMarks(cty.NewValueMarks(SensativeMark))
	}

	if pv.IsNull() {
		return cty.NullVal(cty.DynamicPseudoType)
	}

	if pv.IsComputed() {
		return cty.UnknownVal(cty.DynamicPseudoType)
	}

	switch {
	case pv.IsBool():
		return cty.BoolVal(pv.AsBool())

	case pv.IsString():
		return cty.StringVal(pv.AsString())

	case pv.IsNumber():
		return cty.NumberFloatVal(pv.AsNumber())

	case pv.IsArray():
		arr := pv.AsArray()
		if arr.Len() == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		vals := make([]cty.Value, arr.Len())
		for i, elem := range arr.All {
			vals[i] = PropertyValueToCty(elem)
		}
		return cty.TupleVal(vals)

	case pv.IsMap():
		obj := pv.AsMap()
		if obj.Len() == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value)
		for k, v := range obj.All {
			vals[string(k)] = PropertyValueToCty(v)
		}
		return cty.ObjectVal(vals)

	case pv.IsAsset():
		return cty.CapsuleVal(eval.AssetCapsuleType, pv.AsAsset())

	case pv.IsArchive():
		return cty.CapsuleVal(eval.ArchiveCapsuleType, pv.AsArchive())

	default:
		return cty.NullVal(cty.DynamicPseudoType)
	}
}

// PropertyMapToCty converts a Pulumi PropertyMap to a cty.Value object.
func PropertyMapToCty(pm property.Map) cty.Value {
	if pm.Len() == 0 {
		return cty.EmptyObjectVal
	}

	vals := make(map[string]cty.Value)
	for k, v := range pm.All {
		vals[string(k)] = PropertyValueToCty(v)
	}
	return cty.ObjectVal(vals)
}

// CtyToPropertyMap converts a cty.Value object to a Pulumi PropertyMap.
func CtyToPropertyMap(val cty.Value) (property.Map, error) {
	if val.IsNull() || !val.IsKnown() {
		return property.Map{}, nil
	}

	if !val.Type().IsObjectType() && !val.Type().IsMapType() {
		return property.Map{}, fmt.Errorf("expected object or map type, got %s", val.Type().FriendlyName())
	}

	pm := make(map[string]property.Value)
	for it := val.ElementIterator(); it.Next(); {
		keyVal, elemVal := it.Element()
		key := keyVal.AsString()
		pv, err := CtyToPropertyValue(elemVal)
		if err != nil {
			return property.Map{}, fmt.Errorf("converting property %q: %w", key, err)
		}
		pm[key] = pv
	}
	return property.NewMap(pm), nil
}

// MakeSecret wraps a PropertyValue as a secret.
func MakeSecret(pv resource.PropertyValue) resource.PropertyValue {
	return resource.MakeSecret(pv)
}

// MakeComputed wraps a PropertyValue as computed/unknown.
func MakeComputed(pv resource.PropertyValue) resource.PropertyValue {
	return resource.MakeComputed(pv)
}
