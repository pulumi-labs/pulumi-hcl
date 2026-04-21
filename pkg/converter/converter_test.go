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

package converter

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestPropertiesOf(t *testing.T) {
	t.Parallel()

	objType := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "foo", Type: schema.StringType},
		},
	}

	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, propertiesOf(nil))
	})
	t.Run("object", func(t *testing.T) {
		assert.Equal(t, objType.Properties, propertiesOf(objType))
	})
	t.Run("non-object", func(t *testing.T) {
		assert.Nil(t, propertiesOf(schema.StringType))
	})
	t.Run("optional-object", func(t *testing.T) {
		assert.Equal(t, objType.Properties, propertiesOf(&schema.OptionalType{ElementType: objType}))
	})
	t.Run("array-not-unwrapped", func(t *testing.T) {
		assert.Nil(t, propertiesOf(&schema.ArrayType{ElementType: objType}))
	})
}

func TestElementTypeOf(t *testing.T) {
	t.Parallel()

	t.Run("nil", func(t *testing.T) {
		assert.Nil(t, elementTypeOf(nil))
	})
	t.Run("map", func(t *testing.T) {
		assert.Equal(t, schema.StringType, elementTypeOf(&schema.MapType{ElementType: schema.StringType}))
	})
	t.Run("array", func(t *testing.T) {
		assert.Equal(t, schema.IntType, elementTypeOf(&schema.ArrayType{ElementType: schema.IntType}))
	})
	t.Run("non-container", func(t *testing.T) {
		assert.Nil(t, elementTypeOf(schema.StringType))
	})
}

func TestSchemaAwareTraversalAttrs(t *testing.T) {
	t.Parallel()

	nestedObj := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "nested_output", Type: schema.StringType},
		},
	}
	topProps := []*schema.Property{
		{Name: "the_output", Type: &schema.MapType{
			ElementType: &schema.ArrayType{
				ElementType: nestedObj,
			},
		}},
	}

	trav := hcl.Traversal{
		hcl.TraverseRoot{Name: "first"},
		hcl.TraverseAttr{Name: "the_output"},
		hcl.TraverseIndex{Key: cty.StringVal("someKey")},
		hcl.TraverseIndex{Key: cty.NumberIntVal(0)},
		hcl.TraverseAttr{Name: "nested_output"},
	}

	result := schemaAwareTraversalAttrs(trav, topProps)
	require.Len(t, result, 5)
	assert.Equal(t, "the_output", result[1].(hcl.TraverseAttr).Name)
	assert.Equal(t, "nested_output", result[4].(hcl.TraverseAttr).Name)
}

func TestSchemaAwareTraversalAttrs_CamelCase(t *testing.T) {
	t.Parallel()

	nestedObj := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "nestedOutput", Type: schema.StringType},
		},
	}
	topProps := []*schema.Property{
		{Name: "theOutput", Type: &schema.MapType{
			ElementType: &schema.ArrayType{
				ElementType: nestedObj,
			},
		}},
	}

	trav := hcl.Traversal{
		hcl.TraverseRoot{Name: "first"},
		hcl.TraverseAttr{Name: "the_output"},
		hcl.TraverseIndex{Key: cty.StringVal("someKey")},
		hcl.TraverseIndex{Key: cty.NumberIntVal(0)},
		hcl.TraverseAttr{Name: "nested_output"},
	}

	result := schemaAwareTraversalAttrs(trav, topProps)
	require.Len(t, result, 5)
	assert.Equal(t, "theOutput", result[1].(hcl.TraverseAttr).Name)
	assert.Equal(t, "nestedOutput", result[4].(hcl.TraverseAttr).Name)
}

func TestBlocksToObjectAttrs(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hcl      string
		props    []*schema.Property
		expected string
	}{
		{
			name: "snake_case attrs preserved",
			hcl: `outer {
  outer_attr = "hello"
}`,
			props: []*schema.Property{
				{Name: "outer", Type: &schema.ArrayType{
					ElementType: &schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "outer_attr", Type: schema.StringType},
						},
					},
				}},
			},
			expected: "{\n  outer = [{\n    outer_attr = \"hello\"\n  }]\n}",
		},
		{
			name: "camelCase attrs from snake_case input",
			hcl: `outer {
  outer_attr = "hello"
}`,
			props: []*schema.Property{
				{Name: "outer", Type: &schema.ArrayType{
					ElementType: &schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "outerAttr", Type: schema.StringType},
						},
					},
				}},
			},
			expected: "{\n  outer = [{\n    outerAttr = \"hello\"\n  }]\n}",
		},
		{
			name: "nested blocks with attrs",
			hcl: `outer {
  outer_attr = "hello"
  inner {
    inner_value = "world"
  }
}`,
			props: []*schema.Property{
				{Name: "outer", Type: &schema.ArrayType{
					ElementType: &schema.ObjectType{
						Properties: []*schema.Property{
							{Name: "outer_attr", Type: schema.StringType},
							{Name: "inner", Type: &schema.ArrayType{
								ElementType: &schema.ObjectType{
									Properties: []*schema.Property{
										{Name: "inner_value", Type: schema.StringType},
									},
								},
							}},
						},
					},
				}},
			},
			expected: "{\n  outer = [{\n    outer_attr = \"hello\"\n    inner = [{\n      inner_value = \"world\"\n    }]\n  }]\n}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			src := []byte(tt.hcl)
			file, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{})
			require.False(t, diags.HasErrors(), diags.Error())

			ft := &fileTransformer{src: src}
			result := ft.blocksToObjectAttrs(file.Body.(*hclsyntax.Body).Blocks, tt.props)
			assert.Equal(t, tt.expected, string(hclwrite.TokensForObject(result).Bytes()))
		})
	}
}

func TestTransformFunctionCall(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		hcl      string
		expected string
	}{
		{
			name:     "pcl supported function passes through",
			hcl:      `value = join(", ", split(",", "a,b,c"))`,
			expected: `join(", ", split(",", "a,b,c"))`,
		},
		{
			name:     "hcl function mapped to pcl equivalent",
			hcl:      `value = base64encode("hello")`,
			expected: `toBase64("hello")`,
		},
		{
			name:     "hcl-only function wrapped in notImplemented",
			hcl:      `value = upper("hello")`,
			expected: `notImplemented("upper(\"hello\")")`,
		},
		{
			name:     "hcl-only function with nested args wrapped in notImplemented",
			hcl:      `value = lower(format("Hello %s", "world"))`,
			expected: `notImplemented("lower(format(\"Hello %s\", \"world\"))")`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := []byte(tt.name + " {\n  " + tt.hcl + "\n}\n")
			file, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{})
			require.False(t, diags.HasErrors(), diags.Error())

			ft := &fileTransformer{src: src}
			body := file.Body.(*hclsyntax.Body)
			require.Len(t, body.Blocks, 1)
			require.Contains(t, body.Blocks[0].Body.Attributes, "value")

			tokens := ft.transformExpr(body.Blocks[0].Body.Attributes["value"].Expr)
			assert.Equal(t, tt.expected, string(tokens.Bytes()))
		})
	}
}

// TestEjectDataBlockWithForEach verifies that an HCL program containing a data
// block with `for_each` (the shape produced by the PCL → HCL codegen for invokes
// that close over range.*) round-trips back to valid PCL. The inlined invoke
// picks up the resource's range iterator via `range.value`, and the per-instance
// indexing (`[each.key]`) collapses into the single inlined invoke expression.
func TestEjectDataBlockWithForEach(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Item": {
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"test:index:echo": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"input": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	loader := testutil.NewMockReferenceLoader(t, testSchema)

	src := []byte(`terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  input = each.value
}

resource "test_item" "inbound" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  value = data.test_echo.invoke_0[each.key].result
}
`)

	out := hclwrite.NewEmptyFile()
	diags, err := transformHCLFileToPCL(t.Context(), src, "main.hcl", out.Body(), loader, nil)
	require.NoError(t, err)
	require.False(t, diags.HasErrors(), diags.Error())

	expected := `resource "inbound" "test:index:Item" {
  value = invoke("test:index:echo", {
    input = range.value
  }).result
  options {
    range = {
      "a" = "alpha"
      "b" = "bravo"
    }
  }
}

`
	assert.Equal(t, expected, string(out.Bytes()))
}

// TestEjectInvokeInListComprehension feeds in the HCL shape produced by the
// codegen when a PCL invoke inside a list comprehension is hoisted: a data
// block with for_each over the comprehension's collection, plus a
// comprehension that indexes the data source by its key variable. The eject
// should inline the invoke back inside the comprehension, with references to
// each.value / each.key rewritten to the comprehension's iteration
// variables — NOT to range.* (which would be unbound inside a comprehension).
func TestEjectInvokeInListComprehension(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Functions: map[string]schema.FunctionSpec{
			"test:index:echo": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"input": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	loader := testutil.NewMockReferenceLoader(t, testSchema)
	src := []byte(`terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_echo" "invoke_0" {
  for_each = {
    "a" = "alpha"
    "b" = "bravo"
  }
  input = each.value
}

output "results" {
  value = [for k, v in {
    "a" = "alpha"
    "b" = "bravo"
  } : data.test_echo.invoke_0[k].result]
}
`)

	out := hclwrite.NewEmptyFile()
	diags, err := transformHCLFileToPCL(t.Context(), src, "main.hcl", out.Body(), loader, nil)
	require.NoError(t, err)
	require.False(t, diags.HasErrors(), diags.Error())

	expected := `output "results" {
  value = [for k, v in {
    "a" = "alpha"
    "b" = "bravo"
    } : invoke("test:index:echo", {
      input = v
  }).result]
}

`
	assert.Equal(t, expected, string(out.Bytes()))
}

func TestTransformSplatExpr(t *testing.T) {
	t.Parallel()

	// Schema: myRes has a property "detailItems" (camelCase) of type
	// list(object({ nestedValue: string })).
	// In HCL this would be written as "detail_items" and "nested_value".
	innerObj := &schema.ObjectType{
		Properties: []*schema.Property{
			{Name: "nestedValue", Type: schema.StringType},
		},
	}

	tests := []struct {
		name     string
		hcl      string
		schemas  map[string]*schema.Resource
		expected string
	}{
		{
			name: "splat with camelCase schema property",
			hcl:  `value = my_res.source.detail_items[*].nested_value`,
			schemas: map[string]*schema.Resource{
				"my_res": {
					Properties: []*schema.Property{
						{Name: "detailItems", Type: &schema.ArrayType{
							ElementType: innerObj,
						}},
					},
				},
			},
			expected: `source.detailItems[*].nestedValue`,
		},
		{
			name: "splat with snake_case schema property",
			hcl:  `value = my_res.source.detail_items[*].nested_value`,
			schemas: map[string]*schema.Resource{
				"my_res": {
					Properties: []*schema.Property{
						{Name: "detail_items", Type: &schema.ArrayType{
							ElementType: &schema.ObjectType{
								Properties: []*schema.Property{
									{Name: "nested_value", Type: schema.StringType},
								},
							},
						}},
					},
				},
			},
			expected: `source.detail_items[*].nested_value`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			src := []byte(tt.name + " {\n  " + tt.hcl + "\n}\n")
			file, diags := hclsyntax.ParseConfig(src, "test.hcl", hcl.Pos{})
			require.False(t, diags.HasErrors(), diags.Error())

			ft := &fileTransformer{
				src:             src,
				knownHCLTypes:   map[string]bool{"my_res": true},
				resourceSchemas: tt.schemas,
			}

			body := file.Body.(*hclsyntax.Body)
			require.Len(t, body.Blocks, 1)
			require.Contains(t, body.Blocks[0].Body.Attributes, "value")

			tokens := ft.transformExpr(body.Blocks[0].Body.Attributes["value"].Expr)
			assert.Equal(t, tt.expected, string(tokens.Bytes()))
		})
	}
}
