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

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestSnakeCaseFromCamelCase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		input, expected string
	}{
		{"foo", "foo"},
		{"fooBar", "foo_bar"},
		{"FOO", "foo"},
		{"ec2", "ec2"},
		{"EC2", "ec2"},
		{"fooBARBuzz", "foo_bar_buzz"},
		{"e2e", "e2e"},
	}

	for _, tt := range tests {
		t.Run("", func(t *testing.T) {
			t.Parallel()
			assert.Equalf(t, tt.expected, snakeCaseFromCamelCase(tt.input),
				"snakeCaseFromCamelCase(%q)", tt.input)
		})
	}
}

func TestCtyToResourceInputs(t *testing.T) {
	t.Setenv("TEST_ENV_PORT", "9000")
	t.Setenv("TEST_ENV_ENABLED", "true")

	tests := []struct {
		name       string
		properties []*schema.Property
		input      cty.Value
		expected   resource.PropertyMap
	}{
		{
			name: "simple string property",
			properties: []*schema.Property{
				{
					Name: "name",
					Type: schema.StringType,
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("test-resource"),
			}),
			expected: resource.PropertyMap{
				"name": resource.NewStringProperty("test-resource"),
			},
		},
		{
			name: "boolean and number primitives",
			properties: []*schema.Property{
				{Name: "enabled", Type: schema.BoolType},
				{Name: "count", Type: schema.NumberType},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"enabled": cty.BoolVal(true),
				"count":   cty.NumberIntVal(42),
			}),
			expected: resource.PropertyMap{
				"enabled": resource.NewBoolProperty(true),
				"count":   resource.NewNumberProperty(42),
			},
		},
		{
			name: "object with name translation from snake_case",
			properties: []*schema.Property{
				{
					Name: "containerPort",
					Type: &schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "portNumber", Type: schema.NumberType},
							{Name: "protocol", Type: schema.StringType},
						},
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"container_port": cty.ObjectVal(map[string]cty.Value{
					"port_number": cty.NumberIntVal(8080),
					"protocol":    cty.StringVal("TCP"),
				}),
			}),
			expected: resource.PropertyMap{
				"containerPort": resource.NewObjectProperty(resource.PropertyMap{
					"portNumber": resource.NewNumberProperty(8080),
					"protocol":   resource.NewStringProperty("TCP"),
				}),
			},
		},
		{
			name: "map without name translation",
			properties: []*schema.Property{
				{
					Name: "tags",
					Type: &schema.MapType{ElementType: schema.StringType},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"tags": cty.MapVal(map[string]cty.Value{
					"snake_case_key": cty.StringVal("value1"),
					"another_key":    cty.StringVal("value2"),
				}),
			}),
			expected: resource.PropertyMap{
				"tags": resource.NewObjectProperty(resource.PropertyMap{
					"snake_case_key": resource.NewStringProperty("value1"),
					"another_key":    resource.NewStringProperty("value2"),
				}),
			},
		},
		{
			name: "array of primitives",
			properties: []*schema.Property{
				{
					Name: "ports",
					Type: &schema.ArrayType{ElementType: schema.NumberType},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"ports": cty.ListVal([]cty.Value{
					cty.NumberIntVal(80),
					cty.NumberIntVal(443),
					cty.NumberIntVal(8080),
				}),
			}),
			expected: resource.PropertyMap{
				"ports": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewNumberProperty(80),
					resource.NewNumberProperty(443),
					resource.NewNumberProperty(8080),
				}),
			},
		},
		{
			name: "array of objects with name translation",
			properties: []*schema.Property{
				{
					Name: "endpoints",
					Type: &schema.ArrayType{
						ElementType: &schema.ObjectType{
							Properties: []*schema.Property{
								{Name: "hostName", Type: schema.StringType},
								{Name: "portNumber", Type: schema.NumberType},
							},
						},
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"endpoints": cty.ListVal([]cty.Value{
					cty.ObjectVal(map[string]cty.Value{
						"host_name":   cty.StringVal("api.example.com"),
						"port_number": cty.NumberIntVal(443),
					}),
					cty.ObjectVal(map[string]cty.Value{
						"host_name":   cty.StringVal("db.example.com"),
						"port_number": cty.NumberIntVal(5432),
					}),
				}),
			}),
			expected: resource.PropertyMap{
				"endpoints": resource.NewArrayProperty([]resource.PropertyValue{
					resource.NewObjectProperty(resource.PropertyMap{
						"hostName":   resource.NewStringProperty("api.example.com"),
						"portNumber": resource.NewNumberProperty(443),
					}),
					resource.NewObjectProperty(resource.PropertyMap{
						"hostName":   resource.NewStringProperty("db.example.com"),
						"portNumber": resource.NewNumberProperty(5432),
					}),
				}),
			},
		},
		{
			name: "static default value float64",
			properties: []*schema.Property{
				{Name: "name", Type: schema.StringType},
				{
					Name: "port",
					Type: schema.NumberType,
					DefaultValue: &schema.DefaultValue{
						Value: 8080.0,
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("my-service"),
			}),
			expected: resource.PropertyMap{
				"name": resource.NewStringProperty("my-service"),
				"port": resource.NewNumberProperty(8080),
			},
		},
		{
			name: "static default value int",
			properties: []*schema.Property{
				{Name: "name", Type: schema.StringType},
				{
					Name: "maxConnections",
					Type: schema.NumberType,
					DefaultValue: &schema.DefaultValue{
						Value: 100,
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("my-service"),
			}),
			expected: resource.PropertyMap{
				"name":           resource.NewStringProperty("my-service"),
				"maxConnections": resource.NewNumberProperty(100),
			},
		},
		{
			name: "static default value string",
			properties: []*schema.Property{
				{
					Name: "region",
					Type: schema.StringType,
					DefaultValue: &schema.DefaultValue{
						Value: "us-west-2",
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{}),
			expected: resource.PropertyMap{
				"region": resource.NewStringProperty("us-west-2"),
			},
		},
		{
			name: "static default value boolean",
			properties: []*schema.Property{
				{
					Name: "autoScale",
					Type: schema.BoolType,
					DefaultValue: &schema.DefaultValue{
						Value: true,
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{}),
			expected: resource.PropertyMap{
				"autoScale": resource.NewBoolProperty(true),
			},
		},
		{
			name: "environment variable default overrides static default",
			properties: []*schema.Property{
				{Name: "name", Type: schema.StringType},
				{
					Name: "port",
					Type: schema.NumberType,
					DefaultValue: &schema.DefaultValue{
						Environment: []string{"TEST_ENV_PORT"},
						Value:       8080,
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("my-service"),
			}),
			expected: resource.PropertyMap{
				"name": resource.NewStringProperty("my-service"),
				"port": resource.NewNumberProperty(9000),
			},
		},
		{
			name: "environment variable default for boolean",
			properties: []*schema.Property{
				{
					Name: "enabled",
					Type: schema.BoolType,
					DefaultValue: &schema.DefaultValue{
						Environment: []string{"TEST_ENV_ENABLED"},
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{}),
			expected: resource.PropertyMap{
				"enabled": resource.NewBoolProperty(true),
			},
		},
		{
			name: "secret property",
			properties: []*schema.Property{
				{Name: "password", Type: schema.StringType, Secret: true},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"password": cty.StringVal("super-secret"),
			}),
			expected: resource.PropertyMap{
				"password": resource.MakeSecret(resource.NewStringProperty("super-secret")),
			},
		},
		{
			name: "missing property without default not in output",
			properties: []*schema.Property{
				{Name: "name", Type: schema.StringType},
				{Name: "optionalValue", Type: schema.StringType},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("test"),
			}),
			expected: resource.PropertyMap{
				"name": resource.NewStringProperty("test"),
			},
		},
		{
			name: "deeply nested objects with name translation",
			properties: []*schema.Property{
				{
					Name: "metadata",
					Type: &schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "resourceName", Type: schema.StringType},
							{
								Name: "nestedConfig",
								Type: &schema.ObjectType{
									Properties: []*schema.Property{
										{Name: "maxRetries", Type: schema.NumberType},
										{Name: "timeoutSeconds", Type: schema.NumberType},
									},
								},
							},
						},
					},
				},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"metadata": cty.ObjectVal(map[string]cty.Value{
					"resource_name": cty.StringVal("my-resource"),
					"nested_config": cty.ObjectVal(map[string]cty.Value{
						"max_retries":     cty.NumberIntVal(3),
						"timeout_seconds": cty.NumberIntVal(30),
					}),
				}),
			}),
			expected: resource.PropertyMap{
				"metadata": resource.NewObjectProperty(resource.PropertyMap{
					"resourceName": resource.NewStringProperty("my-resource"),
					"nestedConfig": resource.NewObjectProperty(resource.PropertyMap{
						"maxRetries":     resource.NewNumberProperty(3),
						"timeoutSeconds": resource.NewNumberProperty(30),
					}),
				}),
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ctyToResourceInputs(tt.input, &schema.Resource{
				Token:           "pkg:mod:Name",
				InputProperties: tt.properties,
			})
			require.NoError(t, err)

			// A map may not be reflect.DeepEqual equal, but still be semantically equivalent. If it's not
			// semantically equivalent, we assert it's equal for a good debugging experience.
			if !assert.True(t, tt.expected.DeepEquals(result)) {
				assert.Equal(t, tt.expected, result)
			}
		})
	}
}

func TestCtyToPropertyValue_Primitives(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

	t.Run("list", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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
		t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

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
			t.Parallel()

			result := PropertyValueToCty(tt.input)
			if !result.RawEquals(tt.expected) {
				t.Errorf("expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestPropertyValueToCty_Collections(t *testing.T) {
	t.Parallel()

	t.Run("array", func(t *testing.T) {
		t.Parallel()

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
		t.Parallel()

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
	t.Parallel()

	inner := resource.NewStringProperty("secret-value")
	input := resource.MakeSecret(inner)

	result := PropertyValueToCty(input)

	if !result.IsMarked() {
		t.Fatal("expected value to be marked")
	}
	unmarked, marks := result.Unmark()
	if unmarked.AsString() != "secret-value" {
		t.Errorf("expected secret-value, got %v", unmarked.AsString())
	}
	if _, ok := marks["sensitive"]; !ok {
		t.Error("expected sensitive mark")
	}
}

func TestCtyToPropertyMap(t *testing.T) {
	t.Parallel()

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
	t.Parallel()

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
	t.Parallel()

	// Test that converting cty -> property -> cty preserves values
	original := cty.ObjectVal(map[string]cty.Value{
		"name":  cty.StringVal("test"),
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
