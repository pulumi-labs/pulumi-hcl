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

package transform

import (
	"testing"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/zclconf/go-cty/cty"
)

func TestCtyToPropertyValue_Primitives(t *testing.T) {
	tests := []struct {
		name     string
		input    cty.Value
		expected resource.PropertyValue
	}{
		{
			name:     "null",
			input:    cty.NullVal(cty.String),
			expected: resource.NewNullProperty(),
		},
		{
			name:     "bool true",
			input:    cty.BoolVal(true),
			expected: resource.NewBoolProperty(true),
		},
		{
			name:     "bool false",
			input:    cty.BoolVal(false),
			expected: resource.NewBoolProperty(false),
		},
		{
			name:     "string",
			input:    cty.StringVal("hello"),
			expected: resource.NewStringProperty("hello"),
		},
		{
			name:     "number int",
			input:    cty.NumberIntVal(42),
			expected: resource.NewNumberProperty(42),
		},
		{
			name:     "number float",
			input:    cty.NumberFloatVal(3.14),
			expected: resource.NewNumberProperty(3.14),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CtyToPropertyValue(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.DeepEquals(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestCtyToPropertyValue_Collections(t *testing.T) {
	t.Run("list", func(t *testing.T) {
		input := cty.ListVal([]cty.Value{
			cty.StringVal("a"),
			cty.StringVal("b"),
			cty.StringVal("c"),
		})

		result, err := CtyToPropertyValue(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsArray() {
			t.Fatal("expected array")
		}

		arr := result.ArrayValue()
		if len(arr) != 3 {
			t.Errorf("expected 3 elements, got %d", len(arr))
		}
		if arr[0].StringValue() != "a" || arr[1].StringValue() != "b" || arr[2].StringValue() != "c" {
			t.Error("unexpected array contents")
		}
	})

	t.Run("map", func(t *testing.T) {
		// Maps in cty require homogeneous types, so use all strings
		input := cty.MapVal(map[string]cty.Value{
			"key1": cty.StringVal("value1"),
			"key2": cty.StringVal("value2"),
		})

		result, err := CtyToPropertyValue(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsObject() {
			t.Fatal("expected object")
		}

		obj := result.ObjectValue()
		if obj["key1"].StringValue() != "value1" {
			t.Errorf("expected key1=value1, got %v", obj["key1"])
		}
		if obj["key2"].StringValue() != "value2" {
			t.Errorf("expected key2=value2, got %v", obj["key2"])
		}
	})

	t.Run("object", func(t *testing.T) {
		input := cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("test"),
			"port": cty.NumberIntVal(8080),
		})

		result, err := CtyToPropertyValue(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsObject() {
			t.Fatal("expected object")
		}

		obj := result.ObjectValue()
		if obj["name"].StringValue() != "test" {
			t.Errorf("expected name=test, got %v", obj["name"])
		}
		if obj["port"].NumberValue() != 8080 {
			t.Errorf("expected port=8080, got %v", obj["port"])
		}
	})

	t.Run("tuple", func(t *testing.T) {
		input := cty.TupleVal([]cty.Value{
			cty.StringVal("a"),
			cty.NumberIntVal(1),
			cty.BoolVal(true),
		})

		result, err := CtyToPropertyValue(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsArray() {
			t.Fatal("expected array")
		}

		arr := result.ArrayValue()
		if len(arr) != 3 {
			t.Errorf("expected 3 elements, got %d", len(arr))
		}
	})

	t.Run("set", func(t *testing.T) {
		input := cty.SetVal([]cty.Value{
			cty.StringVal("a"),
			cty.StringVal("b"),
		})

		result, err := CtyToPropertyValue(input)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		if !result.IsArray() {
			t.Fatal("expected array")
		}

		arr := result.ArrayValue()
		if len(arr) != 2 {
			t.Errorf("expected 2 elements, got %d", len(arr))
		}
	})
}

func TestCtyToPropertyValue_Nested(t *testing.T) {
	input := cty.ObjectVal(map[string]cty.Value{
		"tags": cty.MapVal(map[string]cty.Value{
			"env": cty.StringVal("prod"),
		}),
		"ports": cty.ListVal([]cty.Value{
			cty.NumberIntVal(80),
			cty.NumberIntVal(443),
		}),
	})

	result, err := CtyToPropertyValue(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if !result.IsObject() {
		t.Fatal("expected object")
	}

	obj := result.ObjectValue()
	tags := obj["tags"]
	if !tags.IsObject() {
		t.Fatal("expected tags to be object")
	}
	if tags.ObjectValue()["env"].StringValue() != "prod" {
		t.Error("expected tags.env=prod")
	}

	ports := obj["ports"]
	if !ports.IsArray() {
		t.Fatal("expected ports to be array")
	}
	if len(ports.ArrayValue()) != 2 {
		t.Error("expected 2 ports")
	}
}

func TestPropertyValueToCty_Primitives(t *testing.T) {
	tests := []struct {
		name     string
		input    resource.PropertyValue
		expected cty.Value
	}{
		{
			name:     "null",
			input:    resource.NewNullProperty(),
			expected: cty.NullVal(cty.DynamicPseudoType),
		},
		{
			name:     "bool",
			input:    resource.NewBoolProperty(true),
			expected: cty.BoolVal(true),
		},
		{
			name:     "string",
			input:    resource.NewStringProperty("hello"),
			expected: cty.StringVal("hello"),
		},
		{
			name:     "number",
			input:    resource.NewNumberProperty(42),
			expected: cty.NumberFloatVal(42),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := PropertyValueToCty(tt.input)
			if !result.RawEquals(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPropertyValueToCty_Collections(t *testing.T) {
	t.Run("array", func(t *testing.T) {
		input := resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewStringProperty("a"),
			resource.NewStringProperty("b"),
		})

		result := PropertyValueToCty(input)

		// Should be a tuple
		if !result.Type().IsTupleType() {
			t.Fatal("expected tuple type")
		}

		vals := result.AsValueSlice()
		if len(vals) != 2 {
			t.Errorf("expected 2 elements, got %d", len(vals))
		}
		if vals[0].AsString() != "a" || vals[1].AsString() != "b" {
			t.Error("unexpected values")
		}
	})

	t.Run("object", func(t *testing.T) {
		input := resource.NewObjectProperty(resource.PropertyMap{
			"key": resource.NewStringProperty("value"),
		})

		result := PropertyValueToCty(input)

		if !result.Type().IsObjectType() {
			t.Fatal("expected object type")
		}

		val := result.GetAttr("key")
		if val.AsString() != "value" {
			t.Errorf("expected value, got %v", val.AsString())
		}
	})
}

func TestPropertyValueToCty_Secret(t *testing.T) {
	inner := resource.NewStringProperty("secret-value")
	input := resource.MakeSecret(inner)

	result := PropertyValueToCty(input)

	// Secrets are unwrapped
	if result.AsString() != "secret-value" {
		t.Errorf("expected secret-value, got %v", result.AsString())
	}
}

func TestCtyToPropertyMap(t *testing.T) {
	input := cty.ObjectVal(map[string]cty.Value{
		"name":    cty.StringVal("test"),
		"enabled": cty.BoolVal(true),
	})

	result, err := CtyToPropertyMap(input)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if result["name"].StringValue() != "test" {
		t.Errorf("expected name=test")
	}
	if result["enabled"].BoolValue() != true {
		t.Errorf("expected enabled=true")
	}
}

func TestPropertyMapToCty(t *testing.T) {
	input := resource.PropertyMap{
		"name":    resource.NewStringProperty("test"),
		"enabled": resource.NewBoolProperty(true),
	}

	result := PropertyMapToCty(input)

	if !result.Type().IsObjectType() {
		t.Fatal("expected object type")
	}

	if result.GetAttr("name").AsString() != "test" {
		t.Error("expected name=test")
	}
	if result.GetAttr("enabled").True() != true {
		t.Error("expected enabled=true")
	}
}

func TestRoundTrip(t *testing.T) {
	// Test that converting cty -> property -> cty preserves values
	original := cty.ObjectVal(map[string]cty.Value{
		"name": cty.StringVal("test"),
		"count": cty.NumberIntVal(5),
		"tags": cty.MapVal(map[string]cty.Value{
			"env": cty.StringVal("prod"),
		}),
	})

	pv, err := CtyToPropertyValue(original)
	if err != nil {
		t.Fatalf("CtyToPropertyValue error: %v", err)
	}

	result := PropertyValueToCty(pv)

	// Check individual fields since types may differ slightly
	if result.GetAttr("name").AsString() != "test" {
		t.Error("name mismatch")
	}
	if result.GetAttr("count").AsBigFloat().String() != "5" {
		t.Error("count mismatch")
	}
	if result.GetAttr("tags").GetAttr("env").AsString() != "prod" {
		t.Error("tags.env mismatch")
	}
}
