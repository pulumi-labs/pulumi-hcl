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

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils/rapidresource"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils/rapidschema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"pgregory.net/rapid"
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
		expected   property.Map
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
			expected: property.NewMap(map[string]property.Value{
				"name": property.New("test-resource"),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"enabled": property.New(true),
				"count":   property.New(float64(42)),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"containerPort": property.New(property.NewMap(map[string]property.Value{
					"portNumber": property.New(float64(8080)),
					"protocol":   property.New("TCP"),
				})),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"tags": property.New(property.NewMap(map[string]property.Value{
					"snake_case_key": property.New("value1"),
					"another_key":    property.New("value2"),
				})),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"ports": property.New(property.NewArray([]property.Value{
					property.New(float64(80)),
					property.New(float64(443)),
					property.New(float64(8080)),
				})),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"endpoints": property.New(property.NewArray([]property.Value{
					property.New(property.NewMap(map[string]property.Value{
						"hostName":   property.New("api.example.com"),
						"portNumber": property.New(float64(443)),
					})),
					property.New(property.NewMap(map[string]property.Value{
						"hostName":   property.New("db.example.com"),
						"portNumber": property.New(float64(5432)),
					})),
				})),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"name": property.New("my-service"),
				"port": property.New(float64(8080)),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"name":           property.New("my-service"),
				"maxConnections": property.New(float64(100)),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"region": property.New("us-west-2"),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"autoScale": property.New(true),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"name": property.New("my-service"),
				"port": property.New(float64(9000)),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"enabled": property.New(true),
			}),
		},
		{
			name: "secret property",
			properties: []*schema.Property{
				{Name: "password", Type: schema.StringType, Secret: true},
			},
			input: cty.ObjectVal(map[string]cty.Value{
				"password": cty.StringVal("super-secret"),
			}),
			expected: property.NewMap(map[string]property.Value{
				"password": property.New("super-secret").WithSecret(true),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"name": property.New("test"),
			}),
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
			expected: property.NewMap(map[string]property.Value{
				"metadata": property.New(property.NewMap(map[string]property.Value{
					"resourceName": property.New("my-resource"),
					"nestedConfig": property.New(property.NewMap(map[string]property.Value{
						"maxRetries":     property.New(float64(3)),
						"timeoutSeconds": property.New(float64(30)),
					})),
				})),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result, err := ctyToResourceInputs(tt.input, &schema.Resource{
				Token:           "pkg:mod:Name",
				InputProperties: tt.properties,
			}, nil)
			require.NoError(t, err)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// A marked for_each container (e.g. carrying a DepMark from an unknown
// attribute) must not panic ElementIterator.
func TestEvalDynamicBlocks_MarkedForEach(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "fake" "f" {
  dynamic "settings" {
    for_each = local.vpcs
    content {
      name = settings.value.name
    }
  }
}`)
	file, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())

	body := file.Body.(*hclsyntax.Body)
	var resourceBody hcl.Body
	for _, b := range body.Blocks {
		if b.Type == "resource" {
			resourceBody = b.Body
			break
		}
	}
	require.NotNil(t, resourceBody)

	r := &schema.Resource{
		Token: "fake:index:F",
		InputProperties: []*schema.Property{{
			Name: "settings",
			Type: &schema.ArrayType{ElementType: &schema.ObjectType{
				Properties: []*schema.Property{{Name: "name", Type: schema.StringType}},
			}},
		}},
	}

	markedForEach := cty.MapVal(map[string]cty.Value{
		"primary": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("p"),
		}),
	}).WithMarks(cty.NewValueMarks(eval.DepMark("urn:pulumi:dev::p::aws:ec2/vpc:Vpc::test")))

	evalFn := func(_ resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
		if extraVars == nil {
			return markedForEach, nil
		}
		return expr.Value(&hcl.EvalContext{Variables: extraVars})
	}

	_, diags = EvalResourceWithSchema(resourceBody, r, nil, evalFn)
	require.False(t, diags.HasErrors(), "unexpected diags: %v", diags)
}

// A sensitive for_each container must propagate the mark to each.key /
// each.value so attributes derived from them become Pulumi secrets.
func TestEvalDynamicBlocks_SensitiveForEach(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "fake" "f" {
  dynamic "settings" {
    for_each = local.vpcs
    content {
      name = settings.value.name
    }
  }
}`)
	file, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())

	body := file.Body.(*hclsyntax.Body)
	var resourceBody hcl.Body
	for _, b := range body.Blocks {
		if b.Type == "resource" {
			resourceBody = b.Body
			break
		}
	}
	require.NotNil(t, resourceBody)

	r := &schema.Resource{
		Token: "fake:index:F",
		InputProperties: []*schema.Property{{
			Name: "settings",
			Type: &schema.ArrayType{ElementType: &schema.ObjectType{
				Properties: []*schema.Property{{Name: "name", Type: schema.StringType}},
			}},
		}},
	}

	sensitiveForEach := cty.MapVal(map[string]cty.Value{
		"primary": cty.ObjectVal(map[string]cty.Value{
			"name": cty.StringVal("p"),
		}),
	}).WithMarks(cty.NewValueMarks(eval.SensitiveMark))

	evalFn := func(_ resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
		if extraVars == nil {
			return sensitiveForEach, nil
		}
		return expr.Value(&hcl.EvalContext{Variables: extraVars})
	}

	out, diags := EvalResourceWithSchema(resourceBody, r, nil, evalFn)
	require.False(t, diags.HasErrors(), "unexpected diags: %v", diags)

	expected := property.NewMap(map[string]property.Value{
		"settings": property.New(property.NewArray([]property.Value{
			property.New(property.NewMap(map[string]property.Value{
				"name": property.New("p").WithSecret(true),
			})),
		})),
	})
	assert.Equal(t, expected, out)
}

func TestCtyToPropertyValue_Primitives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    cty.Value
		expected property.Value
	}{
		{
			name:     "null",
			input:    cty.NullVal(cty.String),
			expected: property.New(property.Null),
		},
		{
			name:     "bool true",
			input:    cty.BoolVal(true),
			expected: property.New(true),
		},
		{
			name:     "bool false",
			input:    cty.BoolVal(false),
			expected: property.New(false),
		},
		{
			name:     "string",
			input:    cty.StringVal("hello"),
			expected: property.New("hello"),
		},
		{
			name:     "number int",
			input:    cty.NumberIntVal(42),
			expected: property.New(float64(42)),
		},
		{
			name:     "number float",
			input:    cty.NumberFloatVal(3.14),
			expected: property.New(3.14),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, err := CtyToPropertyValue(tt.input)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !result.Equals(tt.expected) {
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

		arr := result.AsArray().AsSlice()
		if len(arr) != 3 {
			t.Errorf("expected 3 elements, got %d", len(arr))
		}
		if arr[0].AsString() != "a" || arr[1].AsString() != "b" || arr[2].AsString() != "c" {
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

		if !result.IsMap() {
			t.Fatal("expected object")
		}

		obj := result.AsMap().AsMap()
		if obj["key1"].AsString() != "value1" {
			t.Errorf("expected key1=value1, got %v", obj["key1"])
		}
		if obj["key2"].AsString() != "value2" {
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

		if !result.IsMap() {
			t.Fatal("expected object")
		}

		obj := result.AsMap().AsMap()
		if obj["name"].AsString() != "test" {
			t.Errorf("expected name=test, got %v", obj["name"])
		}
		if obj["port"].AsNumber() != 8080 {
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

		arr := result.AsArray().AsSlice()
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

		arr := result.AsArray().AsSlice()
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

	if !result.IsMap() {
		t.Fatal("expected object")
	}

	obj := result.AsMap().AsMap()
	tags := obj["tags"]
	if !tags.IsMap() {
		t.Fatal("expected tags to be object")
	}
	if tags.AsMap().AsMap()["env"].AsString() != "prod" {
		t.Error("expected tags.env=prod")
	}

	ports := obj["ports"]
	if !ports.IsArray() {
		t.Fatal("expected ports to be array")
	}
	if len(ports.AsArray().AsSlice()) != 2 {
		t.Error("expected 2 ports")
	}
}

func TestPropertyValueToCty_Primitives(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		input    property.Value
		expected cty.Value
	}{
		{
			name:     "null",
			input:    property.New(property.Null),
			expected: cty.NullVal(cty.DynamicPseudoType),
		},
		{
			name:     "bool",
			input:    property.New(true),
			expected: cty.BoolVal(true),
		},
		{
			name:     "string",
			input:    property.New("hello"),
			expected: cty.StringVal("hello"),
		},
		{
			name:     "number",
			input:    property.New(float64(42)),
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

		input := property.New(property.NewArray([]property.Value{
			property.New("a"),
			property.New("b"),
		}))

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

		input := property.New(property.NewMap(map[string]property.Value{
			"key": property.New("value"),
		}))

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

	input := property.New("secret-value").WithSecret(true)

	result := PropertyValueToCty(input)

	if !result.IsMarked() {
		t.Fatal("expected value to be marked")
	}
	unmarked, marks := result.Unmark()
	if unmarked.AsString() != "secret-value" {
		t.Errorf("expected secret-value, got %v", unmarked.AsString())
	}
	if _, ok := marks[eval.SensitiveMark]; !ok {
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

	if result.Get("name").AsString() != "test" {
		t.Errorf("expected name=test")
	}
	if result.Get("enabled").AsBool() != true {
		t.Errorf("expected enabled=true")
	}
}

func TestPropertyMapToCty(t *testing.T) {
	t.Parallel()

	input := property.NewMap(map[string]property.Value{
		"name":    property.New("test"),
		"enabled": property.New(true),
	})

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

// TestResourceOutputToCtyDoesNotErrorOnValidValues exercises ResourceOutputToCty across
// rapid-drawn schemas and output values.
//
// Since all property bags conform to the schema, they should all be able to be translated
// to [cty.Value] bags.
func TestResourceOutputToCtyDoesNotErrorOnValidValues(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(rt *rapid.T) {
		objectCycles := func(p *schema.Package) bool { return !hasObjectCycle(p) }
		pkg := rapidschema.Package().Filter(objectCycles).Draw(rt, "pkg")
		for _, res := range pkg.Resources {
			if res.IsProvider {
				continue
			}
			outputs := rapidresource.ResourceProperties(res).Draw(rt, "outputs:"+res.Token)
			_, err := ResourceOutputToCty(outputs, res, nil, false)
			require.NoError(t, err)
		}
	})
}

// hasObjectCycle reports whether any ObjectType in the package's type graph
// reaches itself via property types. transform.ctyTypeFromType recurses
// unguardedly through ObjectType.Properties, so cyclic types blow the stack
// before any list-element-type check could fire.
func hasObjectCycle(pkg *schema.Package) bool {
	visited := map[schema.Type]bool{}
	var walk func(t schema.Type, stack map[schema.Type]bool) bool
	walk = func(t schema.Type, stack map[schema.Type]bool) bool {
		if t == nil {
			return false
		}
		if stack[t] {
			return true
		}
		if visited[t] {
			return false
		}
		stack[t] = true
		defer func() {
			delete(stack, t)
			visited[t] = true
		}()
		switch tt := t.(type) {
		case *schema.OptionalType:
			return walk(tt.ElementType, stack)
		case *schema.ArrayType:
			return walk(tt.ElementType, stack)
		case *schema.MapType:
			return walk(tt.ElementType, stack)
		case *schema.ObjectType:
			for _, p := range tt.Properties {
				if walk(p.Type, stack) {
					return true
				}
			}
		case *schema.UnionType:
			if walk(tt.DefaultType, stack) {
				return true
			}
			for _, et := range tt.ElementTypes {
				if walk(et, stack) {
					return true
				}
			}
		}
		return false
	}
	for _, r := range pkg.Resources {
		for _, p := range r.Properties {
			if walk(p.Type, map[schema.Type]bool{}) {
				return true
			}
		}
	}
	return false
}

func TestResourceOutputToCtyUnionTypeCollapse(t *testing.T) {
	t.Parallel()

	obj1 := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "fooBar", Type: schema.StringType},
		},
	}

	obj2 := &schema.MapType{ElementType: schema.StringType}

	union := &schema.UnionType{
		ElementTypes: []schema.Type{obj1, obj2},
	}

	nested := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "p", Type: union},
		},
	}

	res := &schema.Resource{
		Token: "test:index:R",
		Properties: []*schema.Property{
			{Name: "p", Type: nested},
		},
	}

	test := func(t *testing.T, key string, expected cty.Value) {
		outputs := property.NewMap(map[string]property.Value{
			"p": property.New(map[string]property.Value{
				"p": property.New(map[string]property.Value{
					key: property.New("hello"),
				}),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		assert.Equal(t, map[string]cty.Value{
			"p": cty.ObjectVal(map[string]cty.Value{
				"p": expected,
			}),
		}, r)
	}

	t.Run("object", func(t *testing.T) {
		t.Parallel()

		test(t, "fooBar", cty.ObjectVal(map[string]cty.Value{
			"foo_bar": cty.StringVal("hello"),
		}))
	})

	t.Run("map", func(t *testing.T) {
		t.Parallel()

		test(t, "someKey", cty.MapVal(map[string]cty.Value{
			"someKey": cty.StringVal("hello"),
		}))
	})
}

// TestResourceOutputToCtyUnionTypeCollapseNested exercises union resolution
// where the union sits underneath arrays/maps and contains itself nested
// object/map members. The selector has to descend through the carrier
// collections AND through the candidate types to find a match.
func TestResourceOutputToCtyUnionTypeCollapseNested(t *testing.T) {
	t.Parallel()

	obj := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "fooBar", Type: schema.StringType},
		},
	}
	mp := &schema.MapType{ElementType: schema.StringType}
	union := &schema.UnionType{ElementTypes: []schema.Type{obj, mp}}

	t.Run("array_of_union", func(t *testing.T) {
		t.Parallel()

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "items", Type: &schema.ArrayType{ElementType: union}},
			},
		}
		outputs := property.NewMap(map[string]property.Value{
			"items": property.New([]property.Value{
				property.New(map[string]property.Value{"fooBar": property.New("hi")}),
				property.New(map[string]property.Value{"loose": property.New("yo")}),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		// Object{foo_bar:String} and Map<String> unify to Map<String>, so the
		// outer array can stay as a clean cty.ListVal of cty.MapVal.
		assert.Equal(t, map[string]cty.Value{
			"items": cty.ListVal([]cty.Value{
				cty.MapVal(map[string]cty.Value{"foo_bar": cty.StringVal("hi")}),
				cty.MapVal(map[string]cty.Value{"loose": cty.StringVal("yo")}),
			}),
		}, r)
	})

	t.Run("map_of_union", func(t *testing.T) {
		t.Parallel()

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "lookup", Type: &schema.MapType{ElementType: union}},
			},
		}
		outputs := property.NewMap(map[string]property.Value{
			"lookup": property.New(map[string]property.Value{
				"a": property.New(map[string]property.Value{"fooBar": property.New("hi")}),
				"b": property.New(map[string]property.Value{"x": property.New("y")}),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		assert.Equal(t, map[string]cty.Value{
			"lookup": cty.MapVal(map[string]cty.Value{
				"a": cty.MapVal(map[string]cty.Value{"foo_bar": cty.StringVal("hi")}),
				"b": cty.MapVal(map[string]cty.Value{"x": cty.StringVal("y")}),
			}),
		}, r)
	})

	t.Run("array_of_map_of_union", func(t *testing.T) {
		t.Parallel()

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "deep", Type: &schema.ArrayType{
					ElementType: &schema.MapType{ElementType: union},
				}},
			},
		}
		outputs := property.NewMap(map[string]property.Value{
			"deep": property.New([]property.Value{
				property.New(map[string]property.Value{
					"k1": property.New(map[string]property.Value{"fooBar": property.New("hi")}),
					"k2": property.New(map[string]property.Value{"fooBar": property.New("ho")}),
				}),
				property.New(map[string]property.Value{
					"k3": property.New(map[string]property.Value{"free": property.New("form")}),
				}),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		// After union resolution every leaf becomes Map<String>, so both the
		// inner maps and the outer array can stay homogeneous.
		assert.Equal(t, map[string]cty.Value{
			"deep": cty.ListVal([]cty.Value{
				cty.MapVal(map[string]cty.Value{
					"k1": cty.MapVal(map[string]cty.Value{"foo_bar": cty.StringVal("hi")}),
					"k2": cty.MapVal(map[string]cty.Value{"foo_bar": cty.StringVal("ho")}),
				}),
				cty.MapVal(map[string]cty.Value{
					"k3": cty.MapVal(map[string]cty.Value{"free": cty.StringVal("form")}),
				}),
			}),
		}, r)
	})

	t.Run("map_of_union_heterogeneous", func(t *testing.T) {
		t.Parallel()

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "mixed", Type: &schema.MapType{ElementType: union}},
			},
		}
		outputs := property.NewMap(map[string]property.Value{
			"mixed": property.New(map[string]property.Value{
				"a": property.New(map[string]property.Value{"fooBar": property.New("x")}),
				"b": property.New(map[string]property.Value{"free": property.New("y")}),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		// Object{foo_bar:String} and Map<String> unify to Map<String>, so the
		// outer collection can stay as a clean cty.MapVal of cty.MapVal.
		assert.Equal(t, map[string]cty.Value{
			"mixed": cty.MapVal(map[string]cty.Value{
				"a": cty.MapVal(map[string]cty.Value{"foo_bar": cty.StringVal("x")}),
				"b": cty.MapVal(map[string]cty.Value{"free": cty.StringVal("y")}),
			}),
		}, r)
	})

	t.Run("nested_union_disambiguated_by_inner_value", func(t *testing.T) {
		t.Parallel()

		// Both members are objects with a property "p", but the inner type
		// differs. The selector must look at the inner value to choose.
		innerString := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "p", Type: schema.StringType},
			},
		}
		innerBool := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "p", Type: schema.BoolType},
			},
		}
		uu := &schema.UnionType{ElementTypes: []schema.Type{innerString, innerBool}}

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "v", Type: uu},
			},
		}

		t.Run("string_branch", func(t *testing.T) {
			t.Parallel()

			outputs := property.NewMap(map[string]property.Value{
				"v": property.New(map[string]property.Value{"p": property.New("hi")}),
			})
			r, err := ResourceOutputToCty(outputs, res, nil, false)
			require.NoError(t, err)
			assert.Equal(t, map[string]cty.Value{
				"v": cty.ObjectVal(map[string]cty.Value{"p": cty.StringVal("hi")}),
			}, r)
		})

		t.Run("bool_branch", func(t *testing.T) {
			t.Parallel()

			outputs := property.NewMap(map[string]property.Value{
				"v": property.New(map[string]property.Value{"p": property.New(true)}),
			})
			r, err := ResourceOutputToCty(outputs, res, nil, false)
			require.NoError(t, err)
			assert.Equal(t, map[string]cty.Value{
				"v": cty.ObjectVal(map[string]cty.Value{"p": cty.BoolVal(true)}),
			}, r)
		})
	})

	t.Run("required_property_missing_falls_back_to_map", func(t *testing.T) {
		t.Parallel()

		// objBothRequired requires both x and y. The value omits y, so it
		// cannot be a valid objBothRequired. The only fitting member is
		// Map<string>. A matcher that only checks that value keys belong
		// to the object's properties — without verifying that *required*
		// properties of the object are present in the value — would
		// incorrectly pick objBothRequired.
		objBothRequired := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "x", Type: schema.StringType},
				{Name: "y", Type: schema.StringType},
			},
		}
		mp := &schema.MapType{ElementType: schema.StringType}
		uu := &schema.UnionType{ElementTypes: []schema.Type{objBothRequired, mp}}

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "v", Type: uu},
			},
		}

		outputs := property.NewMap(map[string]property.Value{
			"v": property.New(map[string]property.Value{
				"x": property.New("hi"),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		assert.Equal(t, map[string]cty.Value{
			"v": cty.MapVal(map[string]cty.Value{
				"x": cty.StringVal("hi"),
			}),
		}, r)
	})

	t.Run("optional_property_value_must_type_check", func(t *testing.T) {
		t.Parallel()

		// objA has a required `x` and an optional `extras` typed as int.
		// The value provides `extras` as a string — that must disqualify
		// objA. Map<string> is the only remaining valid member.
		objA := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "x", Type: schema.StringType},
				{Name: "extras", Type: &schema.OptionalType{ElementType: schema.IntType}},
			},
		}
		mp := &schema.MapType{ElementType: schema.StringType}
		uu := &schema.UnionType{ElementTypes: []schema.Type{objA, mp}}

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "v", Type: uu},
			},
		}

		outputs := property.NewMap(map[string]property.Value{
			"v": property.New(map[string]property.Value{
				"x":      property.New("hi"),
				"extras": property.New("not-an-int"),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		assert.Equal(t, map[string]cty.Value{
			"v": cty.MapVal(map[string]cty.Value{
				"x":      cty.StringVal("hi"),
				"extras": cty.StringVal("not-an-int"),
			}),
		}, r)
	})

	t.Run("optional_present_picks_object_over_object_without_it", func(t *testing.T) {
		t.Parallel()

		// objA only has x. objB has x required and extras optional.
		// When the value carries `extras`, only objB can describe it.
		// objA is listed first, but the matcher must reject it because
		// `extras` is not a property of objA.
		objA := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "x", Type: schema.StringType},
			},
		}
		objB := &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "x", Type: schema.StringType},
				{Name: "extras", Type: &schema.OptionalType{ElementType: schema.StringType}},
			},
		}
		uu := &schema.UnionType{ElementTypes: []schema.Type{objA, objB}}

		res := &schema.Resource{
			Token: "test:index:R",
			Properties: []*schema.Property{
				{Name: "v", Type: uu},
			},
		}

		outputs := property.NewMap(map[string]property.Value{
			"v": property.New(map[string]property.Value{
				"x":      property.New("hi"),
				"extras": property.New("ho"),
			}),
		})

		r, err := ResourceOutputToCty(outputs, res, nil, false)
		require.NoError(t, err)
		assert.Equal(t, map[string]cty.Value{
			"v": cty.ObjectVal(map[string]cty.Value{
				"x":      cty.StringVal("hi"),
				"extras": cty.StringVal("ho"),
			}),
		}, r)
	})
}

func TestResourceOutputToCtyJSONType(t *testing.T) {
	t.Parallel()

	inner := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "blob", Type: schema.JSONType},
		},
	}
	res := &schema.Resource{
		Token: "test:index:R",
		Properties: []*schema.Property{
			{Name: "wrapper", Type: inner},
		},
	}

	outputs := property.NewMap(map[string]property.Value{
		"wrapper": property.New(map[string]property.Value{
			"blob": property.New(map[string]property.Value{
				"k": property.New("v"),
			}),
		}),
	})

	r, err := ResourceOutputToCty(outputs, res, nil, false)
	require.NoError(t, err)
	assert.Equal(t, map[string]cty.Value{
		"wrapper": cty.ObjectVal(map[string]cty.Value{
			"blob": cty.MapVal(map[string]cty.Value{
				"k": cty.StringVal("v"),
			}),
		}),
	}, r)
}

// TestCtyEqualsConst exercises every Go type that bindConstValue is
// documented to produce (string, bool, int32, float64). Earlier versions
// of selectUnionMemberByConst only compared strings, so other-typed
// discriminators silently failed to match.
func TestCtyEqualsConst(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		val   cty.Value
		c     any
		match bool
	}{
		{"string match", cty.StringVal("a"), "a", true},
		{"string mismatch", cty.StringVal("a"), "b", false},
		{"string vs bool", cty.StringVal("true"), true, false},

		{"bool match", cty.True, true, true},
		{"bool mismatch", cty.False, true, false},
		{"bool vs string", cty.True, "true", false},

		{"int32 match", cty.NumberIntVal(7), int32(7), true},
		{"int32 mismatch", cty.NumberIntVal(7), int32(8), false},
		{"int32 vs fractional", cty.NumberFloatVal(7.5), int32(7), false},
		{"int32 vs string-typed", cty.StringVal("7"), int32(7), false},

		{"float64 match", cty.NumberFloatVal(1.5), 1.5, true},
		{"float64 mismatch", cty.NumberFloatVal(1.5), 2.5, false},
		{"float64 vs string-typed", cty.StringVal("1.5"), 1.5, false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.match, CtyEqualsConst(tt.val, tt.c))
		})
	}
}

// TestSelectUnionMemberByConst_IntDiscriminator proves the non-string
// path end-to-end: a union of two object variants whose only difference
// is an int32-valued `version` const is correctly resolved.
func TestSelectUnionMemberByConst_IntDiscriminator(t *testing.T) {
	t.Parallel()

	v1 := &schema.ObjectType{
		Token: "test:index:V1",
		Properties: []*schema.Property{
			{Name: "version", Type: schema.IntType, ConstValue: int32(1)},
			{Name: "a", Type: schema.StringType},
		},
	}
	v2 := &schema.ObjectType{
		Token: "test:index:V2",
		Properties: []*schema.Property{
			{Name: "version", Type: schema.IntType, ConstValue: int32(2)},
			{Name: "b", Type: schema.StringType},
		},
	}
	u := &schema.UnionType{ElementTypes: []schema.Type{v1, v2}}

	picked, err := selectUnionMemberByConst(cty.ObjectVal(map[string]cty.Value{
		"version": cty.NumberIntVal(2),
		"b":       cty.StringVal("hello"),
	}), u)
	require.NoError(t, err)
	assert.Same(t, v2, picked)

	picked, err = selectUnionMemberByConst(cty.ObjectVal(map[string]cty.Value{
		"version": cty.NumberIntVal(1),
		"a":       cty.StringVal("hello"),
	}), u)
	require.NoError(t, err)
	assert.Same(t, v1, picked)

	_, err = selectUnionMemberByConst(cty.ObjectVal(map[string]cty.Value{
		"version": cty.NumberIntVal(99),
	}), u)
	var unrecognized *unrecognizedDiscriminatorError
	require.ErrorAs(t, err, &unrecognized)
	assert.Equal(t, []string{"1", "2"}, unrecognized.Allowed)
	assert.Equal(t, "99", unrecognized.Actual)
}
