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
	"encoding/json"
	"fmt"
	"math/big"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/zclconf/go-cty/cty"
)

func CtyToResourcePropertyValue(val cty.Value, field string, r *schema.Resource) (resource.PropertyValue, error) {
	return CtyToPropertyValue(val)
}

// CtyToPropertyValue converts a cty.Value to a Pulumi PropertyValue.
func CtyToPropertyValue(val cty.Value) (resource.PropertyValue, error) {
	// Handle sensitive-marked values by unwrapping, converting, and wrapping as secret.
	if val.IsMarked() {
		unmarked, marks := val.Unmark()
		pv, err := CtyToPropertyValue(unmarked)
		if err != nil {
			return pv, err
		}
		if _, isSensitive := marks["sensitive"]; isSensitive {
			return resource.MakeSecret(pv), nil
		}
		return pv, nil
	}

	if val.IsNull() {
		return resource.NewNullProperty(), nil
	}

	if !val.IsKnown() {
		// Unknown values are represented as computed in Pulumi
		return resource.MakeComputed(resource.NewStringProperty("")), nil
	}

	typ := val.Type()

	switch {
	case typ == cty.Bool:
		return resource.NewBoolProperty(val.True()), nil

	case typ == cty.String:
		return resource.NewStringProperty(val.AsString()), nil

	case typ == cty.Number:
		bf := val.AsBigFloat()
		// Try to convert to int64 first
		if bf.IsInt() {
			if i64, acc := bf.Int64(); acc == big.Exact {
				return resource.NewNumberProperty(float64(i64)), nil
			}
		}
		// Fall back to float64
		f64, _ := bf.Float64()
		return resource.NewNumberProperty(f64), nil

	case typ.IsListType() || typ.IsTupleType():
		return ctyListToPropertyValue(val)

	case typ.IsSetType():
		return ctySetToPropertyValue(val)

	case typ.IsMapType() || typ.IsObjectType():
		return ctyObjectToPropertyValue(val)

	case typ == cty.DynamicPseudoType:
		// Dynamic type - try to infer the type from the underlying value
		return resource.NewNullProperty(), nil

	default:
		// For complex types we don't handle directly, try JSON encoding
		return ctyToPropertyViaJSON(val)
	}
}

// ctyListToPropertyValue converts a cty list/tuple to a Pulumi array.
func ctyListToPropertyValue(val cty.Value) (resource.PropertyValue, error) {
	var arr []resource.PropertyValue
	for it := val.ElementIterator(); it.Next(); {
		_, elemVal := it.Element()
		pv, err := CtyToPropertyValue(elemVal)
		if err != nil {
			return resource.PropertyValue{}, err
		}
		arr = append(arr, pv)
	}
	return resource.NewArrayProperty(arr), nil
}

// ctySetToPropertyValue converts a cty set to a Pulumi array.
func ctySetToPropertyValue(val cty.Value) (resource.PropertyValue, error) {
	var arr []resource.PropertyValue
	for it := val.ElementIterator(); it.Next(); {
		_, elemVal := it.Element()
		pv, err := CtyToPropertyValue(elemVal)
		if err != nil {
			return resource.PropertyValue{}, err
		}
		arr = append(arr, pv)
	}
	return resource.NewArrayProperty(arr), nil
}

// ctyObjectToPropertyValue converts a cty map/object to a Pulumi object.
func ctyObjectToPropertyValue(val cty.Value) (resource.PropertyValue, error) {
	obj := make(resource.PropertyMap)
	for it := val.ElementIterator(); it.Next(); {
		keyVal, elemVal := it.Element()
		key := keyVal.AsString()
		pv, err := CtyToPropertyValue(elemVal)
		if err != nil {
			return resource.PropertyValue{}, err
		}
		obj[resource.PropertyKey(key)] = pv
	}
	return resource.NewObjectProperty(obj), nil
}

// ctyToPropertyViaJSON converts a cty value to PropertyValue via JSON encoding.
func ctyToPropertyViaJSON(val cty.Value) (resource.PropertyValue, error) {
	// Use cty's JSON encoding
	jsonVal, err := json.Marshal(ctyToGo(val))
	if err != nil {
		return resource.PropertyValue{}, fmt.Errorf("encoding value to JSON: %w", err)
	}

	var generic any
	if err := json.Unmarshal(jsonVal, &generic); err != nil {
		return resource.PropertyValue{}, fmt.Errorf("decoding JSON: %w", err)
	}

	return goToPropertyValue(generic)
}

// ctyToGo converts a cty.Value to a Go any.
func ctyToGo(val cty.Value) any {
	if val.IsNull() {
		return nil
	}
	if !val.IsKnown() {
		return nil
	}

	typ := val.Type()

	switch {
	case typ == cty.Bool:
		return val.True()

	case typ == cty.String:
		return val.AsString()

	case typ == cty.Number:
		bf := val.AsBigFloat()
		if bf.IsInt() {
			if i64, acc := bf.Int64(); acc == big.Exact {
				return i64
			}
		}
		f64, _ := bf.Float64()
		return f64

	case typ.IsListType() || typ.IsTupleType() || typ.IsSetType():
		var arr []any
		for it := val.ElementIterator(); it.Next(); {
			_, elemVal := it.Element()
			arr = append(arr, ctyToGo(elemVal))
		}
		return arr

	case typ.IsMapType() || typ.IsObjectType():
		obj := make(map[string]any)
		for it := val.ElementIterator(); it.Next(); {
			keyVal, elemVal := it.Element()
			obj[keyVal.AsString()] = ctyToGo(elemVal)
		}
		return obj

	default:
		return nil
	}
}

// goToPropertyValue converts a Go any to a Pulumi PropertyValue.
func goToPropertyValue(v any) (resource.PropertyValue, error) {
	if v == nil {
		return resource.NewNullProperty(), nil
	}

	switch val := v.(type) {
	case bool:
		return resource.NewBoolProperty(val), nil
	case string:
		return resource.NewStringProperty(val), nil
	case float64:
		return resource.NewNumberProperty(val), nil
	case int:
		return resource.NewNumberProperty(float64(val)), nil
	case int64:
		return resource.NewNumberProperty(float64(val)), nil
	case []any:
		arr := make([]resource.PropertyValue, len(val))
		for i, elem := range val {
			pv, err := goToPropertyValue(elem)
			if err != nil {
				return resource.PropertyValue{}, err
			}
			arr[i] = pv
		}
		return resource.NewArrayProperty(arr), nil
	case map[string]any:
		obj := make(resource.PropertyMap)
		for k, elem := range val {
			pv, err := goToPropertyValue(elem)
			if err != nil {
				return resource.PropertyValue{}, err
			}
			obj[resource.PropertyKey(k)] = pv
		}
		return resource.NewObjectProperty(obj), nil
	default:
		return resource.PropertyValue{}, fmt.Errorf("unsupported type: %T", v)
	}
}

func ResourcePropertyToCty(pv resource.PropertyValue, field resource.PropertyKey, r *schema.Resource) (string, cty.Value) {
	return string(field), PropertyValueToCty(pv)
}

// PropertyValueToCty converts a Pulumi PropertyValue to a cty.Value.
func PropertyValueToCty(pv resource.PropertyValue) cty.Value {
	if pv.IsNull() {
		return cty.NullVal(cty.DynamicPseudoType)
	}

	if pv.IsComputed() {
		return cty.UnknownVal(cty.DynamicPseudoType)
	}

	switch {
	case pv.IsBool():
		return cty.BoolVal(pv.BoolValue())

	case pv.IsString():
		return cty.StringVal(pv.StringValue())

	case pv.IsNumber():
		return cty.NumberFloatVal(pv.NumberValue())

	case pv.IsArray():
		arr := pv.ArrayValue()
		if len(arr) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		vals := make([]cty.Value, len(arr))
		for i, elem := range arr {
			vals[i] = PropertyValueToCty(elem)
		}
		return cty.TupleVal(vals)

	case pv.IsObject():
		obj := pv.ObjectValue()
		if len(obj) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value)
		for k, v := range obj {
			vals[string(k)] = PropertyValueToCty(v)
		}
		return cty.ObjectVal(vals)

	case pv.IsSecret():
		// Convert the inner value and mark it as sensitive
		inner := PropertyValueToCty(pv.SecretValue().Element)
		return inner.Mark("sensitive")

	case pv.IsOutput():
		// For outputs, try to get the known value if available
		output := pv.OutputValue()
		if output.Known {
			return PropertyValueToCty(output.Element)
		}
		return cty.UnknownVal(cty.DynamicPseudoType)

	default:
		return cty.NullVal(cty.DynamicPseudoType)
	}
}

// PropertyMapToCty converts a Pulumi PropertyMap to a cty.Value object.
func PropertyMapToCty(pm resource.PropertyMap) cty.Value {
	if len(pm) == 0 {
		return cty.EmptyObjectVal
	}

	vals := make(map[string]cty.Value)
	for k, v := range pm {
		vals[string(k)] = PropertyValueToCty(v)
	}
	return cty.ObjectVal(vals)
}

// CtyToPropertyMap converts a cty.Value object to a Pulumi PropertyMap.
func CtyToPropertyMap(val cty.Value) (resource.PropertyMap, error) {
	if val.IsNull() || !val.IsKnown() {
		return nil, nil
	}

	if !val.Type().IsObjectType() && !val.Type().IsMapType() {
		return nil, fmt.Errorf("expected object or map type, got %s", val.Type().FriendlyName())
	}

	pm := make(resource.PropertyMap)
	for it := val.ElementIterator(); it.Next(); {
		keyVal, elemVal := it.Element()
		key := keyVal.AsString()
		pv, err := CtyToPropertyValue(elemVal)
		if err != nil {
			return nil, fmt.Errorf("converting property %q: %w", key, err)
		}
		pm[resource.PropertyKey(key)] = pv
	}
	return pm, nil
}

// MakeSecret wraps a PropertyValue as a secret.
func MakeSecret(pv resource.PropertyValue) resource.PropertyValue {
	return resource.MakeSecret(pv)
}

// MakeComputed wraps a PropertyValue as computed/unknown.
func MakeComputed(pv resource.PropertyValue) resource.PropertyValue {
	return resource.MakeComputed(pv)
}
