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

package main

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/codegen"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	hclrun "github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/schemaloader"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestConvertedPCL(t *testing.T) {
	t.Parallel()

	t.Run("function_blocks", func(t *testing.T) {
		t.Parallel()

		pclSource := `output filteredId {
    value = invoke("test:index:getFiltered", {
        name = "my-filter"
        filters = [{
            key = "tag:Name"
            value = "production"
        }, {
            key = "tag:Env"
            value = "prod"
        }]
    }).id
}
`

		testSchema := schema.PackageSpec{
			Name:    "test",
			Version: "1.0.0",
			Functions: map[string]schema.FunctionSpec{
				"test:index:getFiltered": {
					Inputs: &schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
							"filters": {
								TypeSpec: schema.TypeSpec{
									Type: "array",
									Items: &schema.TypeSpec{
										Ref: "#/types/test:index:Filter",
									},
								},
							},
						},
					},
					Outputs: &schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"id": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
			Types: map[string]schema.ComplexTypeSpec{
				"test:index:Filter": {
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Type: "object",
						Properties: map[string]schema.PropertySpec{
							"key":   {TypeSpec: schema.TypeSpec{Type: "string"}},
							"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
		}

		mock := testConvertedPCL(t, pclSource, testSchema)

		require.Len(t, mock.InvokedFunctions, 1)
		assert.Equal(t, "test:index:getFiltered", mock.InvokedFunctions[0].Token)
		assert.Equal(t, property.NewMap(map[string]property.Value{
			"name": property.New("my-filter"),
			"filters": property.New(property.NewArray([]property.Value{
				property.New(property.NewMap(map[string]property.Value{
					"key":   property.New("tag:Name"),
					"value": property.New("production"),
				})),
				property.New(property.NewMap(map[string]property.Value{
					"key":   property.New("tag:Env"),
					"value": property.New("prod"),
				})),
			})),
		}), mock.InvokedFunctions[0].Args)
	})

	t.Run("function_nested_blocks", func(t *testing.T) {
		t.Parallel()

		pclSource := `output result {
    value = invoke("test:index:blockInvoke", {
        outer = [{
            inner = [{
                prop = true
            }, {
                prop = false
            }]
        }, {
            inner = [{
                prop = false
            }, {
                prop = true
            }]
        }]
    }).id
}

output emptyOuter {
    value = invoke("test:index:blockInvoke", {
        outer = []
    }).id
}

output emptyInner {
    value = invoke("test:index:blockInvoke", {
        outer = [{
            inner = []
        }]
    }).id
}
`

		testSchema := schema.PackageSpec{
			Name:    "test",
			Version: "1.0.0",
			Functions: map[string]schema.FunctionSpec{
				"test:index:blockInvoke": {
					Inputs: &schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"outer": {
								TypeSpec: schema.TypeSpec{
									Type:  "array",
									Items: &schema.TypeSpec{Ref: "#/types/test:index:Outer"},
								},
							},
						},
					},
					Outputs: &schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"id": {TypeSpec: schema.TypeSpec{Type: "string"}},
						},
					},
				},
			},
			Types: map[string]schema.ComplexTypeSpec{
				"test:index:Outer": {
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Type: "object",
						Properties: map[string]schema.PropertySpec{
							"inner": {
								TypeSpec: schema.TypeSpec{
									Type:  "array",
									Items: &schema.TypeSpec{Ref: "#/types/test:index:Inner"},
								},
							},
						},
					},
				},
				"test:index:Inner": {
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Type: "object",
						Properties: map[string]schema.PropertySpec{
							"prop": {TypeSpec: schema.TypeSpec{Type: "boolean"}},
						},
					},
				},
			},
		}

		mock := testConvertedPCL(t, pclSource, testSchema)

		assert.ElementsMatch(t, mock.InvokedFunctions, []hclrun.InvokeRequest{
			{
				Token: "test:index:blockInvoke",
				Args:  property.Map{},
			},
			{
				Token: "test:index:blockInvoke",
				Args: property.NewMap(map[string]property.Value{
					"outer": property.New([]property.Value{
						property.New(property.Map{}),
					}),
				}),
			},
			{
				Token: "test:index:blockInvoke",
				Args: property.NewMap(map[string]property.Value{
					"outer": property.New([]property.Value{
						property.New(map[string]property.Value{
							"inner": property.New([]property.Value{
								property.New(map[string]property.Value{"prop": property.New(true)}),
								property.New(map[string]property.Value{"prop": property.New(false)}),
							}),
						}),
						property.New(map[string]property.Value{
							"inner": property.New([]property.Value{
								property.New(map[string]property.Value{"prop": property.New(false)}),
								property.New(map[string]property.Value{"prop": property.New(true)}),
							}),
						}),
					}),
				}),
			},
		})
	})

	t.Run("blocks", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource myServer "test:index:Server" {
    name = "my-server"
    networkRules = [{
        protocol = "tcp"
        port = 443
    }, {
        protocol = "udp"
        port = 53
    }]
}
`

		testSchema := schema.PackageSpec{
			Name:    "test",
			Version: "1.0.0",
			Resources: map[string]schema.ResourceSpec{
				"test:index:Server": {
					InputProperties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"networkRules": {
							TypeSpec: schema.TypeSpec{
								Type: "array",
								Items: &schema.TypeSpec{
									Ref: "#/types/test:index:NetworkRule",
								},
							},
						},
					},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{
							"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
							"networkRules": {
								TypeSpec: schema.TypeSpec{
									Type: "array",
									Items: &schema.TypeSpec{
										Ref: "#/types/test:index:NetworkRule",
									},
								},
							},
						},
					},
				},
			},
			Types: map[string]schema.ComplexTypeSpec{
				"test:index:NetworkRule": {
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Type: "object",
						Properties: map[string]schema.PropertySpec{
							"protocol": {TypeSpec: schema.TypeSpec{Type: "string"}},
							"port":     {TypeSpec: schema.TypeSpec{Type: "integer"}},
						},
					},
				},
			},
		}

		mock := testConvertedPCL(t, pclSource, testSchema)

		require.Len(t, mock.RegisteredResources, 2, "expected stack + server")

		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)

		server := mock.RegisteredResources[1]
		assert.Equal(t, "test:index:Server", server.Type)
		assert.Equal(t, "myServer", server.Name)
		assert.Equal(t, property.New("my-server"), server.Inputs.Get("name"))
		assert.Equal(t, property.New(property.NewArray([]property.Value{
			property.New(property.NewMap(map[string]property.Value{
				"protocol": property.New("tcp"),
				"port":     property.New(float64(443)),
			})),
			property.New(property.NewMap(map[string]property.Value{
				"protocol": property.New("udp"),
				"port":     property.New(float64(53)),
			})),
		})), server.Inputs.Get("networkRules"))
	})
}

func TestConvertedPCLRange(t *testing.T) {
	t.Parallel()

	rangeSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Item": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}

	t.Run("range_bool", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource myItem "test:index:Item" {
    options {
        range = true
    }
    name = "static-item"
}
`

		mock := testConvertedPCL(t, pclSource, rangeSchema)

		// With enabled=true (default true in PCL), we should have stack + 1 item
		require.Len(t, mock.RegisteredResources, 2, "expected stack + 1 item")
		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
		assert.Equal(t, "test:index:Item", mock.RegisteredResources[1].Type)
	})

	t.Run("range_count", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource myItem "test:index:Item" {
    options {
        range = 3
    }
    name = "item-${range.value}"
}
`

		mock := testConvertedPCL(t, pclSource, rangeSchema)

		// stack + 3 items
		require.Len(t, mock.RegisteredResources, 4, "expected stack + 3 items")
		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
		for i := 1; i <= 3; i++ {
			assert.Equal(t, "test:index:Item", mock.RegisteredResources[i].Type)
		}
	})

	t.Run("range_map", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource myItem "test:index:Item" {
    options {
        range = {
            a = "alpha"
            b = "bravo"
        }
    }
    name = range.value
}
`

		mock := testConvertedPCL(t, pclSource, rangeSchema)

		// stack + 2 items
		require.Len(t, mock.RegisteredResources, 3, "expected stack + 2 items")
		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
		assert.Equal(t, "test:index:Item", mock.RegisteredResources[1].Type)
		assert.Equal(t, "test:index:Item", mock.RegisteredResources[2].Type)
	})

	t.Run("range_count_ref", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource source "test:index:Item" {
    options {
        range = 2
    }
    name = "src-${range.value}"
}
resource target "test:index:Item" {
    name = "${source[0].name}-ref"
}
`

		mock := testConvertedPCL(t, pclSource, rangeSchema)

		// stack + 2 source items + 1 target
		require.Len(t, mock.RegisteredResources, 4, "expected stack + 2 sources + 1 target")
		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)

		target := mock.RegisteredResources[3]
		assert.Equal(t, "test:index:Item", target.Type)
		assert.Equal(t, property.New("src-0-ref"), target.Inputs.Get("name"))
	})

	t.Run("range_map_ref", func(t *testing.T) {
		t.Parallel()

		pclSource := `resource source "test:index:Item" {
    options {
        range = {
            x = "alpha"
            y = "bravo"
        }
    }
    name = range.value
}
resource target "test:index:Item" {
    name = "${source["x"].name}-ref"
}
`

		mock := testConvertedPCL(t, pclSource, rangeSchema)

		// stack + 2 source items + 1 target
		require.Len(t, mock.RegisteredResources, 4, "expected stack + 2 sources + 1 target")

		target := mock.RegisteredResources[3]
		assert.Equal(t, "test:index:Item", target.Type)
		assert.Equal(t, property.New("alpha-ref"), target.Inputs.Get("name"))
	})

}

// TestStdLookupInlinedInForEachResource checks that a `std:index:*` function (which has a
// TF-builtin equivalent) is hoisted into a top-level `data "std_*"` block instead of
// being inlined as a TF builtin call. When the invoke closes over `range.value` from an
// enclosing for_each resource, the hoisted block has no for_each and the rewritten
// `each.value` reference is unbound at plan time.
//
// The fix is to reverse the `std:index:lookup` → `lookup(...)` mapping,
// emitting an inline TF function call:
//
//	resource "test_item" "inbound" {
//	  for_each = { a = { thing = "alpha" }, b = { thing = "bravo" } }
//	  value    = lookup(each.value, "thing", "none")
//	}
func TestStdLookupInlinedInForEachResource(t *testing.T) {
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
	}
	stdSchema := schema.PackageSpec{
		Name:    "std",
		Version: "1.0.0",
		Functions: map[string]schema.FunctionSpec{
			"std:index:lookup": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"map":     {TypeSpec: schema.TypeSpec{Ref: "pulumi.json#/Any"}},
						"key":     {TypeSpec: schema.TypeSpec{Type: "string"}},
						"default": {TypeSpec: schema.TypeSpec{Ref: "pulumi.json#/Any"}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Ref: "pulumi.json#/Any"}},
					},
				},
			},
		},
	}

	pclSource := `resource inbound "test:index:Item" {
    options {
        range = {
            a = { thing = "alpha" }
            b = { thing = "bravo" }
        }
    }
    value = invoke("std:index:lookup", {
        map     = range.value
        key     = "thing"
        default = "none"
    }).result
}
`

	// No InvokeHandler: std:index:lookup must be inlined as the TF builtin
	// `lookup(...)`, which the engine evaluates natively. If the generator
	// still emits a `data "std_lookup"` block, the engine will either fail
	// to bind `each.value` or fail to resolve the std provider.
	mock := testConvertedPCLWithComponent(t, pclSource, nil, nil, testSchema, stdSchema)

	require.Len(t, mock.RegisteredResources, 3, "expected stack + 2 items")
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	assert.Empty(t, mock.InvokedFunctions,
		"std:index:lookup should be inlined, not sent as an Invoke")

	values := []property.Value{
		mock.RegisteredResources[1].Inputs.Get("value"),
		mock.RegisteredResources[2].Inputs.Get("value"),
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha"),
		property.New("bravo"),
	}, values)
}

// TestCustomInvokeHoistedWithForEach checks a custom provider invoke that has no
// TF-builtin equivalent must still be emitted as a `data` block, but when it closes over
// `range.value` the data block needs its own `for_each` and the resource must reference
// it per-iteration:
//
//	data "test_echo" "invoke_0" {
//	  for_each = { a = "alpha", b = "bravo" }
//	  input    = each.value
//	}
//	resource "test_item" "inbound" {
//	  for_each = data.test_echo.invoke_0
//	  value    = each.value.result
//	}
func TestCustomInvokeHoistedWithForEach(t *testing.T) {
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

	pclSource := `resource inbound "test:index:Item" {
    options {
        range = {
            a = "alpha"
            b = "bravo"
        }
    }
    value = invoke("test:index:echo", {
        input = range.value
    }).result
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	// The invoke must fire once per range entry with the corresponding
	// range.value, not once with an unbound placeholder.
	invokeInputs := make([]property.Value, 0, len(mock.InvokedFunctions))
	for _, inv := range mock.InvokedFunctions {
		assert.Equal(t, "test:index:echo", inv.Token)
		input, ok := inv.Args.GetOk("input")
		require.True(t, ok, "invoke missing `input` arg: %v", inv.Args)
		invokeInputs = append(invokeInputs, input)
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha"),
		property.New("bravo"),
	}, invokeInputs)

	// Each registered resource should receive the per-iteration invoke
	// result.
	require.Len(t, mock.RegisteredResources, 3, "expected stack + 2 items")
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	values := []property.Value{
		mock.RegisteredResources[1].Inputs.Get("value"),
		mock.RegisteredResources[2].Inputs.Get("value"),
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha+"),
		property.New("bravo+"),
	}, values)
}

// TestCustomInvokeHoistedWithCount mirrors TestCustomInvokeHoistedWithForEach
// but uses a numeric range, which maps to `count` instead of `for_each`. The
// hoisted data block should carry a matching `count` and the resource should
// reference it with `[count.index]`.
func TestCustomInvokeHoistedWithCount(t *testing.T) {
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

	pclSource := `resource inbound "test:index:Item" {
    options {
        range = 2
    }
    value = invoke("test:index:echo", {
        input = "item-${range.value}"
    }).result
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	invokeInputs := make([]property.Value, 0, len(mock.InvokedFunctions))
	for _, inv := range mock.InvokedFunctions {
		assert.Equal(t, "test:index:echo", inv.Token)
		input, ok := inv.Args.GetOk("input")
		require.True(t, ok, "invoke missing `input` arg: %v", inv.Args)
		invokeInputs = append(invokeInputs, input)
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("item-0"),
		property.New("item-1"),
	}, invokeInputs)

	require.Len(t, mock.RegisteredResources, 3, "expected stack + 2 items")
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	values := []property.Value{
		mock.RegisteredResources[1].Inputs.Get("value"),
		mock.RegisteredResources[2].Inputs.Get("value"),
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("item-0+"),
		property.New("item-1+"),
	}, values)
}

// TestCustomInvokeAssignedToLocalKeepsName verifies when an invoke is the direct value of
// a top-level local variable (`name = invoke(...)`), the hoisted data block should reuse
// the local's name (`data "<token>" "name"`) rather than the auto-generated
// `invoke_N`. References to the local then rewrite to `data.<token>.<name>.<output>`.
func TestCustomInvokeAssignedToLocalKeepsName(t *testing.T) {
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

	pclSource := `myConfig = invoke("test:index:echo", {
    input = "hello"
})

output result {
    value = myConfig.result
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	require.Len(t, mock.InvokedFunctions, 1)
	assert.Equal(t, "test:index:echo", mock.InvokedFunctions[0].Token)

	result, ok := mock.StackOutputs.GetOk("result")
	require.True(t, ok, "expected 'result' stack output")
	assert.Equal(t, property.New("hello+"), result)
}

// TestCustomInvokeHoistedInListComprehension checks that a custom provider
// invoke that appears inside a PCL list comprehension is hoisted into a data
// block whose `for_each` matches the comprehension's collection, and that the
// reference is spliced back into the list comprehension indexed by the
// comprehension's key variable.
func TestCustomInvokeHoistedInListComprehension(t *testing.T) {
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

	pclSource := `output results {
    value = [for k, v in {
        a = "alpha"
        b = "bravo"
    } : invoke("test:index:echo", {
        input = v
    }).result]
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	invokeInputs := make([]property.Value, 0, len(mock.InvokedFunctions))
	for _, inv := range mock.InvokedFunctions {
		assert.Equal(t, "test:index:echo", inv.Token)
		input, ok := inv.Args.GetOk("input")
		require.True(t, ok, "invoke missing `input` arg: %v", inv.Args)
		invokeInputs = append(invokeInputs, input)
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha"),
		property.New("bravo"),
	}, invokeInputs)

	require.Len(t, mock.RegisteredResources, 1, "expected just the stack")
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok, "missing `results` stack output")
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha+"),
		property.New("bravo+"),
	}, results.AsArray().AsSlice())
}

// TestCustomInvokeHoistedInListComprehensionKeyVar verifies the branch of the
// for-variable rewrite that maps a reference to the comprehension's
// KeyVariable onto `each.key` in the hoisted data block.
func TestCustomInvokeHoistedInListComprehensionKeyVar(t *testing.T) {
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

	pclSource := `output results {
    value = [for k, v in {
        a = "alpha"
        b = "bravo"
    } : invoke("test:index:echo", {
        input = k
    }).result]
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "!"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	invokeInputs := make([]property.Value, 0, len(mock.InvokedFunctions))
	for _, inv := range mock.InvokedFunctions {
		input, ok := inv.Args.GetOk("input")
		require.True(t, ok)
		invokeInputs = append(invokeInputs, input)
	}
	assert.ElementsMatch(t, []property.Value{
		property.New("a"),
		property.New("b"),
	}, invokeInputs)

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.ElementsMatch(t, []property.Value{
		property.New("a!"),
		property.New("b!"),
	}, results.AsArray().AsSlice())
}

// TestStdInvokeInListComprehensionInlined checks that a std:index:* invoke
// with a TF-builtin equivalent is inlined at the reference site (no data
// block is spilled) even when the invoke appears inside a list comprehension
// and closes over the comprehension's value variable.
func TestStdInvokeInListComprehensionInlined(t *testing.T) {
	t.Parallel()

	stdSchema := schema.PackageSpec{
		Name:    "std",
		Version: "1.0.0",
		Functions: map[string]schema.FunctionSpec{
			"std:index:lower": {
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

	pclSource := `output results {
    value = [for _, v in {
        a = "ALPHA"
        b = "BRAVO"
    } : invoke("std:index:lower", {
        input = v
    }).result]
}
`

	mock := testConvertedPCLWithComponent(t, pclSource, nil, nil, stdSchema)

	assert.Empty(t, mock.InvokedFunctions,
		"std:index:lower should be inlined as the TF builtin, not sent as an Invoke")

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha"),
		property.New("bravo"),
	}, results.AsArray().AsSlice())
}

// TestCustomInvokeHoistedInMapComprehension verifies hoisting works for the
// map-result form of a for-comprehension: {for k, v in xs : k => invoke(...)}.
func TestCustomInvokeHoistedInMapComprehension(t *testing.T) {
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

	pclSource := `output results {
    value = {for k, v in {
        a = "alpha"
        b = "bravo"
    } : k => invoke("test:index:echo", {
        input = v
    }).result}
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.Equal(t, property.NewMap(map[string]property.Value{
		"a": property.New("alpha+"),
		"b": property.New("bravo+"),
	}), results.AsMap())
}

// TestCustomInvokeHoistedInNestedComprehension verifies that when an invoke
// inside nested comprehensions references an OUTER comprehension's iteration
// variable, the walk-from-innermost-out loop in collectInvokesInExpr picks
// the outer for-expression as the one whose collection drives the data
// block's for_each.
func TestCustomInvokeHoistedInNestedComprehension(t *testing.T) {
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

	pclSource := `output results {
    value = [for ko, vo in {
        a = "alpha"
    } : [for ki, vi in {
        x = "xylo"
    } : invoke("test:index:echo", {
        input = vo
    }).result]]
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	assert.Len(t, mock.InvokedFunctions, 1,
		"invoke should fire once per outer iteration (not inner × outer)")

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	outer := results.AsArray().AsSlice()
	require.Len(t, outer, 1)
	inner := outer[0].AsArray().AsSlice()
	require.Len(t, inner, 1)
	assert.Equal(t, property.New("alpha+"), inner[0])
}

// TestCustomInvokeHoistedInListComprehensionNoKey exercises the branch where a
// for-comprehension has no KeyVariable. The current codegen tags the invoke
// with the for-expression anyway and emits `for_each = <list>` on the data
// block, but the reference rewrite cannot index the data block (no key in
// scope) — so the per-iteration semantics break. This test documents the
// current behavior; it is expected to fail until the no-key case is fixed.
func TestCustomInvokeHoistedInListComprehensionNoKey(t *testing.T) {
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

	pclSource := `output results {
    value = [for v in ["alpha", "bravo"] : invoke("test:index:echo", {
        input = v
    }).result]
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.ElementsMatch(t, []property.Value{
		property.New("alpha+"),
		property.New("bravo+"),
	}, results.AsArray().AsSlice())
}

// TestCustomInvokeHoistedReferencesOuterForCollection covers a nested for-comprehension
// where an invoke is bound to the innermost for-expression (via its ValueVariable), but
// that for's collection itself references the outer for-expression's iter var.
func TestCustomInvokeHoistedReferencesOuterForCollection(t *testing.T) {
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

	pclSource := `output results {
    value = [for entry in [{filter = "alpha"}, {filter = "bravo"}] :
        [for v in try([entry.filter], []) : invoke("test:index:echo", {
            input = v
        }).result]]
}
`

	testConvertedPCL(t, pclSource, testSchema)
}

// TestCustomInvokeHoistedFourDeepNestedForCollections is the 4-deep analogue of
// TestCustomInvokeHoistedReferencesOuterForCollection: each inner for's
// collection references the iter var of its immediately-enclosing for, so the
// hoisted data block's for_each must flatten through all three outer fors.
func TestCustomInvokeHoistedFourDeepNestedForCollections(t *testing.T) {
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

	pclSource := `output results {
    value = [for a in [{items = [{items = [{items = ["alpha", "bravo"]}]}]}, {items = [{items = [{items = ["charlie"]}]}]}] :
        [for b in try(a.items, []) :
            [for c in try(b.items, []) :
                [for d in try(c.items, []) : invoke("test:index:echo", {
                    input = d
                }).result]]]]
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.Equal(t, property.New([]property.Value{
		property.New([]property.Value{
			property.New([]property.Value{
				property.New([]property.Value{
					property.New("alpha+"),
					property.New("bravo+"),
				}),
			}),
		}),
		property.New([]property.Value{
			property.New([]property.Value{
				property.New([]property.Value{
					property.New("charlie+"),
				}),
			}),
		}),
	}), results)
}

// TestCustomInvokeHoistedOuterForHasKeyVar exercises the branch of
// forExprWrapperTokens that emits `[for k, v in coll : ...]` (rather than the
// no-key form) — i.e. an outer for-comprehension with a KeyVariable that an
// inner hoisted invoke's collection references.
func TestCustomInvokeHoistedOuterForHasKeyVar(t *testing.T) {
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

	pclSource := `output results {
    value = {for ko, vo in {
        a = "alpha"
        b = "bravo"
    } : ko => [for v in [vo, "${ko}-x"] : invoke("test:index:echo", {
        input = v
    }).result]}
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			input, _ := req.Args.GetOk("input")
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"result": property.New(input.AsString() + "+"),
				}),
			}, nil
		},
	}

	mock := testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)

	results, ok := mock.StackOutputs.GetOk("results")
	require.True(t, ok)
	assert.Equal(t, property.New(map[string]property.Value{
		"a": property.New([]property.Value{
			property.New("alpha+"),
			property.New("a-x+"),
		}),
		"b": property.New([]property.Value{
			property.New("bravo+"),
			property.New("b-x+"),
		}),
	}), results)
}

func TestNotImplemented(t *testing.T) {
	t.Parallel()

	generateHCL := func(t *testing.T, pclSource string) string {
		t.Helper()

		loader := schemaloader.New(t)

		p := syntax.NewParser()
		err := p.ParseFile(strings.NewReader(pclSource), "main.pp")
		require.NoError(t, err)
		require.False(t, p.Diagnostics.HasErrors(), p.Diagnostics.Error())

		program, bindDiags, err := pcl.BindProgram(p.Files, pcl.Loader(loader))
		require.NoError(t, err)
		require.False(t, bindDiags.HasErrors(), bindDiags.Error())

		files, genDiags, err := codegen.GenerateProgram(program)
		require.NoError(t, err)
		require.False(t, genDiags.HasErrors(), genDiags.Error())

		generatedHCL := string(files["main.tf"])
		require.NotEmpty(t, generatedHCL)

		hclParser := parser.NewParser()
		_, hclDiags := hclParser.ParseSource("main.tf", files["main.tf"])
		require.False(t, hclDiags.HasErrors(), hclDiags.Error())

		return generatedHCL
	}

	t.Run("known_function", func(t *testing.T) {
		t.Parallel()

		hcl := generateHCL(t, `output result {
    value = notImplemented("upper(\"hello\")")
}
`)
		assert.Equal(t, `output "result" {
  value = upper("hello")
}
`, hcl)
	})

	t.Run("unknown_function", func(t *testing.T) {
		t.Parallel()

		hcl := generateHCL(t, `output result {
    value = notImplemented("mystery_func(\"hello\")")
}
`)
		assert.Equal(t, `output "result" {
  value = notImplemented("mystery_func(\"hello\")")
}
`, hcl)
	})
}

// TestGenerateProgramSkipResourceTypechecking covers the program-generation path used by
// pulumi/pulumi-terraform-bridge when converting third-party Terraform docs. The bridge
// binds with AllowMissingProperties / AllowMissingVariables / SkipResourceTypechecking,
// which can leave pcl.Resource.Schema unset (no schema is loaded for "simple:..." here).
// scopeTraversalTokens used to dereference part.Schema.Properties unconditionally and
// crashed with a nil-pointer panic on this input.
func TestGenerateProgramSkipResourceTypechecking(t *testing.T) {
	t.Parallel()

	const src = `
resource aResource "simple:index/resource:Resource" {
    inputOne = "hello"
}

output someOutput {
    value = aResource.result
}
`
	loader := schemaloader.New(t)

	p := syntax.NewParser()
	require.NoError(t, p.ParseFile(strings.NewReader(src), "main.pp"))
	require.False(t, p.Diagnostics.HasErrors(), p.Diagnostics.Error())

	program, bindDiags, err := pcl.BindProgram(p.Files,
		pcl.Loader(loader),
		pcl.AllowMissingProperties,
		pcl.AllowMissingVariables,
		pcl.SkipResourceTypechecking,
	)
	require.NoError(t, err)
	require.False(t, bindDiags.HasErrors(), bindDiags.Error())

	files, genDiags, err := codegen.GenerateProgram(program)
	require.NoError(t, err)
	require.False(t, genDiags.HasErrors(), genDiags.Error())

	hclParser := parser.NewParser()
	_, hclDiags := hclParser.ParseSource("main.tf", files["main.tf"])
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())
}

// TestInvokeOutput locks in that traversals into an invoke's outputs are rewritten using
// the function's schema.
func TestInvokeOutput(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Sink": {
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"test:index:getInfo": {
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"snake_case_field": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"tagsMap": {
							TypeSpec: schema.TypeSpec{
								Type:                 "object",
								AdditionalProperties: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
	}

	pclSource := `info = invoke("test:index:getInfo", {})

resource "snakeSink" "test:index:Sink" {
    value = info.snake_case_field
}

resource "tagSink" "test:index:Sink" {
    value = info.tagsMap.UserKey
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, _ hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"snake_case_field": property.New("value"),
					"tagsMap": property.New(property.NewMap(map[string]property.Value{
						"UserKey": property.New("v"),
					})),
				}),
			}, nil
		},
	}

	testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)
}

// TestInvokeReturnType covers the case where a function declares its outputs via
// FunctionSpec.ReturnType.
func TestInvokeReturnType(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Sink": {
				InputProperties: map[string]schema.PropertySpec{
					"value": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"test:index:Info": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"snake_case_field": {TypeSpec: schema.TypeSpec{Type: "string"}},
						"tagsMap": {
							TypeSpec: schema.TypeSpec{
								Type:                 "object",
								AdditionalProperties: &schema.TypeSpec{Type: "string"},
							},
						},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"test:index:getInfo": {
				ReturnType: &schema.ReturnTypeSpec{
					TypeSpec: &schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/test:index:Info"},
					},
				},
			},
		},
	}

	pclSource := `info = invoke("test:index:getInfo", {})

resource "snakeSink" "test:index:Sink" {
    value = info[0].snake_case_field
}

resource "tagSink" "test:index:Sink" {
    value = info[0].tagsMap.UserKey
}
`

	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, _ hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{
					"items": property.New([]property.Value{
						property.New(property.NewMap(map[string]property.Value{
							"snake_case_field": property.New("value"),
							"tagsMap": property.New(property.NewMap(map[string]property.Value{
								"UserKey": property.New("v"),
							})),
						})),
					}),
				}),
			}, nil
		},
	}

	testConvertedPCLWithComponent(t, pclSource, nil, monitor, testSchema)
}

func testConvertedPCL(t *testing.T, pclSource string, schemas ...schema.PackageSpec) *testutil.MockResourceMonitor {
	t.Helper()
	return testConvertedPCLWithComponent(t, pclSource, nil, nil, schemas...)
}

func TestInvokeModuleFormat(t *testing.T) {
	t.Parallel()

	pclSource := `ubuntu = invoke("test:mod/getThing:getThing", {
    objectBlocks = [{
        value = true
    }, {
        value = false
    }]
})

output result {
    value = ubuntu.id
}
`

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Meta:    &schema.MetadataSpec{ModuleFormat: "(.*)(?:/[^/]*)"},
		Functions: map[string]schema.FunctionSpec{
			"test:mod/getThing:getThing": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"objectBlocks": {TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Ref: "#/types/test:mod/getThingBlock:getThingBlock"},
						}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"id": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"test:mod/getThingBlock:getThingBlock": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "boolean"}},
					},
					Required: []string{"value"},
				},
			},
		},
	}

	testConvertedPCL(t, pclSource, testSchema)
}

func TestConvertHeredoc(t *testing.T) {
	t.Parallel()

	pclSource := `
resource normal "test:index:Res" {
    prop = <<EON
 hello
world
EON
}

resource indented "test:index:Res" {
    prop = <<-EOI
      hello: ${normal.id}
       world
      EOI
}
`

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Res": {
				InputProperties: map[string]schema.PropertySpec{
					"prop": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
			},
		},
	}

	m := testConvertedPCL(t, pclSource, testSchema)

	var found1, found2 bool
	for _, r := range m.RegisteredResources {
		switch r.Name {
		case "normal":
			assert.Equal(t, property.NewMap(map[string]property.Value{
				"prop": property.New(" hello\nworld\n"),
			}), r.Inputs)
			found1 = true
		case "indented":
			assert.Equal(t, property.NewMap(map[string]property.Value{
				"prop": property.New("hello: normal-id\n world\n"),
			}), r.Inputs)
			found2 = true
		}
	}

	assert.True(t, found1)
	assert.True(t, found2)
}

func TestResourceModuleFormat(t *testing.T) {
	t.Parallel()

	pclSource := `resource ubuntu "test:mod/Thing:Thing" {
    objectBlocks = [{
        value = true
    }, {
        value = false
    }]
}
`

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Meta:    &schema.MetadataSpec{ModuleFormat: "(.*)(?:/[^/]*)"},
		Resources: map[string]schema.ResourceSpec{
			"test:mod/Thing:Thing": {
				InputProperties: map[string]schema.PropertySpec{
					"objectBlocks": {TypeSpec: schema.TypeSpec{
						Type:  "array",
						Items: &schema.TypeSpec{Ref: "#/types/test:mod/ThingBlock:ThingBlock"},
					}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"objectBlocks": {TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Ref: "#/types/test:mod/ThingBlock:ThingBlock"},
						}},
					},
				},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"test:mod/ThingBlock:ThingBlock": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"value": {TypeSpec: schema.TypeSpec{Type: "boolean"}},
					},
					Required: []string{"value"},
				},
			},
		},
	}

	testConvertedPCL(t, pclSource, testSchema)
}

func TestLocalExecProvisioner(t *testing.T) {
	t.Parallel()

	src := `terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = "6.0.0"
    }
  }
}

resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"

  provisioner "local-exec" {
    command     = "echo ${self.ami}"
    working_dir = "/tmp"
  }
}

output "instance_ami" {
  value = aws_instance.web.ami
}`

	awsSchema := schema.PackageSpec{
		Name:    "aws",
		Version: "6.0.0",
		Resources: map[string]schema.ResourceSpec{
			"aws:index:Instance": {
				InputProperties: map[string]schema.PropertySpec{
					"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
					"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}

	loader := schemaloader.New(t, awsSchema)

	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseSource("main.tf", []byte(src))
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())

	mock := &testutil.MockResourceMonitor{}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader:    loader,
	})

	err := engine.Run(t.Context())
	require.NoError(t, err)

	// Expect: stack + aws_instance + command:local:Command provisioner
	require.Len(t, mock.RegisteredResources, 3)

	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	assert.Equal(t, "aws:index:Instance", mock.RegisteredResources[1].Type)
	assert.Equal(t, "web", mock.RegisteredResources[1].Name)
	assert.Equal(t, "command:local:Command", mock.RegisteredResources[2].Type)
	assert.Equal(t, "aws_instance.web-provisioner-0", mock.RegisteredResources[2].Name)

	provInputs := mock.RegisteredResources[2].Inputs
	create, ok := provInputs.GetOk("create")
	require.True(t, ok, "expected 'create' input on provisioner")
	assert.Equal(t, "echo ami-12345", create.AsString())

	dir, ok := provInputs.GetOk("dir")
	require.True(t, ok, "expected 'dir' input on provisioner")
	assert.Equal(t, "/tmp", dir.AsString())

	// Provisioner should depend on the parent resource.
	assert.Equal(t, []string{
		"urn:pulumi:test::project::aws:index:Instance::web",
	}, mock.RegisteredResources[2].Dependencies)

	// Provisioner should be parented to the resource.
	assert.Equal(t,
		"urn:pulumi:test::project::aws:index:Instance::web",
		mock.RegisteredResources[2].Parent,
	)

	// Stack output should reflect the resource's ami.
	ami, ok := mock.StackOutputs.GetOk("instance_ami")
	require.True(t, ok)
	assert.Equal(t, "ami-12345", ami.AsString())
}

// TestModuleVariableResolution reproduces https://github.com/pulumi-labs/pulumi-hcl/issues/77:
// module variable references don't resolve inside module scope.
//
// The bug is that processDataSource always evaluates expressions in the root
// evaluator context instead of the module instance's context. This means
// data source expressions that reference module variables (var.X) fail because
// the root context doesn't contain the module's var namespace.
func TestModuleVariableResolution(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Functions: map[string]schema.FunctionSpec{
			"test:index:getLen": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"items": {TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
			},
		},
	}

	// The component has a variable and a data source (invoke) that references
	// it. The invoke becomes a `data` block in HCL, which exercises
	// processDataSource — the function that fails to use the module instance's
	// eval context.
	componentPCL := `config "items" "list(string)" {
}

itemLen = invoke("test:index:getLen", {
  items = items
}).result

output "result" {
  value = itemLen
}
`
	parentPCL := `component "mod" "./mod" {
  items = ["a", "b", "c"]
}

output "result" {
  value = mod.result
}
`
	monitor := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			if req.Token == "test:index:getLen" {
				items, ok := req.Args.GetOk("items")
				if ok && items.IsArray() {
					return &hclrun.InvokeResponse{
						Return: property.NewMap(map[string]property.Value{
							"result": property.New(float64(items.AsArray().Len())),
						}),
					}, nil
				}
			}
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{}),
			}, nil
		},
	}
	mock := testConvertedPCLWithComponent(t, parentPCL, map[string]string{
		"./mod": componentPCL,
	}, monitor, testSchema)

	result, ok := mock.StackOutputs.GetOk("result")
	require.True(t, ok, "expected 'result' stack output")
	assert.Equal(t, property.New(float64(3)), result)
}

// TestModuleResourceVariableResolution verifies that a resource inside a module
// can reference module variables (var.X) in its inputs.
func TestModuleResourceVariableResolution(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Bucket": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}

	componentPCL := `config "bucketName" "string" {
}

resource "bucket" "test:index:Bucket" {
  name = bucketName
}

output "name" {
  value = bucket.name
}
`
	parentPCL := `component "mod" "./mod" {
  bucketName = "my-bucket"
}

output "name" {
  value = mod.name
}
`
	mock := testConvertedPCLWithComponent(t, parentPCL, map[string]string{
		"./mod": componentPCL,
	}, nil, testSchema)

	result, ok := mock.StackOutputs.GetOk("name")
	require.True(t, ok, "expected 'name' stack output")
	assert.Equal(t, property.New("my-bucket"), result)
}

// TestNestedModuleVariableResolution verifies that a nested module (a module
// called from within another module) can receive inputs from its parent module's
// scope. This reproduces https://github.com/pulumi-labs/pulumi-hcl/issues/78.
func TestNestedModuleVariableResolution(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Bucket": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}

	// The inner module creates a resource using a variable passed from the
	// outer module.
	innerPCL := `config "name" "string" {
}

resource "bucket" "test:index:Bucket" {
  name = "inner(${name})"
}

output "bucketName" {
  value = bucket.name
}
`
	// The outer module receives a variable from the root and forwards it to
	// the inner module.
	outerPCL := `config "name" "string" {
}

component "inner" "./inner" {
  name = "outer(${name})"
}

output "bucketName" {
  value = inner.bucketName
}
`
	parentPCL := `component "outer" "./outer" {
  name = "my-bucket"
}

output "bucketName" {
  value = outer.bucketName
}
`
	mock := testConvertedPCLWithComponent(t, parentPCL, map[string]string{
		"./outer": outerPCL,
		"./inner": innerPCL,
	}, nil, testSchema)

	result, ok := mock.StackOutputs.GetOk("bucketName")
	require.True(t, ok, "expected 'bucketName' stack output")
	assert.Equal(t, property.New("inner(outer(my-bucket))"), result)
}

// TestModuleDataSourceDependencies verifies that data sources inside modules
// correctly track dependencies on resources and other data sources, using the
// module-prefixed keys.
func TestModuleDataSourceDependencies(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Bucket": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"test:index:getLen": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"items": {TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
			},
		},
	}
	loader := schemaloader.New(t, testSchema)

	// The module contains:
	// - A resource (test_bucket.bucket)
	// - A data source that references the resource's name AND has depends_on
	//   pointing to the resource.
	// This exercises the module-prefix dependency tracking in
	// processDataSourceInContext for both expression dependencies and depends_on.
	parentHCL := `
terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

module "mod" {
  source     = "./mod"
  bucketName = "my-bucket"
}

output "result" {
  value = module.mod.result
}
`
	modHCL := `
terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

variable "bucketName" {
  type = string
}

resource "test_bucket" "bucket" {
  name = var.bucketName
}

data "test_getlen" "invoke_0" {
  items      = [test_bucket.bucket.name]
  depends_on = [test_bucket.bucket]
}

locals {
  itemLen = data.test_getlen.invoke_0.result
}

output "result" {
  value = local.itemLen
}
`
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(parentHCL), 0o644))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mod", "main.tf"), []byte(modHCL), 0o644))

	hclParser := parser.NewParser()
	config, diags := hclParser.ParseDirectory(dir)
	require.False(t, diags.HasErrors(), diags.Error())

	mock := &testutil.MockResourceMonitor{
		InvokeHandler: func(_ context.Context, req hclrun.InvokeRequest) (*hclrun.InvokeResponse, error) {
			if req.Token == "test:index:getLen" {
				items, ok := req.Args.GetOk("items")
				if ok && items.IsArray() {
					return &hclrun.InvokeResponse{
						Return: property.NewMap(map[string]property.Value{
							"result": property.New(float64(items.AsArray().Len())),
						}),
					}, nil
				}
			}
			return &hclrun.InvokeResponse{
				Return: property.NewMap(map[string]property.Value{}),
			}, nil
		},
	}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         dir,
		RootDir:         dir,
		SchemaLoader:    loader,
	})

	err := engine.Run(t.Context())
	require.NoError(t, err)

	result, ok := mock.StackOutputs.GetOk("result")
	require.True(t, ok, "expected 'result' stack output")
	assert.Equal(t, property.New(float64(1)), result)
}

// TestModuleScopeIsolation verifies that resources and data sources inside a
// module cannot access variables from the parent scope.
func TestModuleScopeIsolation(t *testing.T) {
	t.Parallel()

	testSchema := schema.PackageSpec{
		Name:    "test",
		Version: "1.0.0",
		Resources: map[string]schema.ResourceSpec{
			"test:index:Bucket": {
				InputProperties: map[string]schema.PropertySpec{
					"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"name": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
		Functions: map[string]schema.FunctionSpec{
			"test:index:getLen": {
				Inputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"items": {TypeSpec: schema.TypeSpec{
							Type:  "array",
							Items: &schema.TypeSpec{Type: "string"},
						}},
					},
				},
				Outputs: &schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"result": {TypeSpec: schema.TypeSpec{Type: "number"}},
					},
				},
			},
		},
	}
	loader := schemaloader.New(t, testSchema)

	// The parent defines a local that the module tries to reference.
	parentHCL := `
terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

locals {
  parentName = "from-parent"
}

module "mod" {
  source = "./mod"
}
`

	t.Run("resource", func(t *testing.T) {
		t.Parallel()

		// The module's resource references local.parentName, which only
		// exists in the parent scope and should not be visible here.
		modHCL := `
terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_bucket" "bucket" {
  name = local.parentName
}
`
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(parentHCL), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "mod"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mod", "main.tf"), []byte(modHCL), 0o644))

		hclParser := parser.NewParser()
		config, diags := hclParser.ParseDirectory(dir)
		require.False(t, diags.HasErrors(), diags.Error())

		mock := &testutil.MockResourceMonitor{}
		engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
			ProjectName:     "test-project",
			StackName:       "dev",
			ResourceMonitor: mock,
			WorkDir:         dir,
			RootDir:         dir,
			SchemaLoader:    loader,
		})

		err := engine.Run(t.Context())
		assert.EqualError(t, err, fmt.Sprintf(
			`%s:12,10-26: unknown node "module.mod.local.parentName"; `,
			filepath.Join(dir, "mod", "main.hcl"),
		))
	})

	t.Run("data_source", func(t *testing.T) {
		t.Parallel()

		// The module's data source references local.parentName, which only
		// exists in the parent scope and should not be visible here.
		modHCL := `
terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

data "test_getlen" "invoke_0" {
  items = [local.parentName]
}
`
		dir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(parentHCL), 0o644))
		require.NoError(t, os.MkdirAll(filepath.Join(dir, "mod"), 0o755))
		require.NoError(t, os.WriteFile(filepath.Join(dir, "mod", "main.tf"), []byte(modHCL), 0o644))

		hclParser := parser.NewParser()
		config, diags := hclParser.ParseDirectory(dir)
		require.False(t, diags.HasErrors(), diags.Error())

		mock := &testutil.MockResourceMonitor{}
		engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
			ProjectName:     "test-project",
			StackName:       "dev",
			ResourceMonitor: mock,
			WorkDir:         dir,
			RootDir:         dir,
			SchemaLoader:    loader,
		})

		err := engine.Run(t.Context())
		assert.EqualError(t, err, fmt.Sprintf(
			`%s:12,12-28: unknown node "module.mod.local.parentName"; `,
			filepath.Join(dir, "mod", "main.hcl"),
		))
	})
}

// testConvertedPCLWithComponent is like testConvertedPCL but supports PCL
// programs that contain component blocks. componentSources maps component
// directory names (e.g. "mod") to their PCL source.
func testConvertedPCLWithComponent(
	t *testing.T, parentPCL string,
	componentSources map[string]string,
	mock *testutil.MockResourceMonitor,
	schemas ...schema.PackageSpec,
) *testutil.MockResourceMonitor {
	t.Helper()

	loader := schemaloader.New(t, schemas...)

	// Build an in-memory ComponentProgramBinder so we don't need files on disk
	// for the PCL binding step. The binder is defined as a variable so the
	// closure can reference itself, enabling nested component resolution.
	var componentBinder pcl.ComponentProgramBinder
	componentBinder = func(args pcl.ComponentProgramBinderArgs) (*pcl.Program, hcl.Diagnostics, error) {
		src, ok := componentSources[args.ComponentSource]
		if !ok {
			return nil, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "unknown component",
				Detail:   args.ComponentSource,
			}}, nil
		}
		p := syntax.NewParser()
		if err := p.ParseFile(strings.NewReader(src), "main.pp"); err != nil {
			return nil, nil, err
		}
		if p.Diagnostics.HasErrors() {
			return nil, p.Diagnostics, nil
		}
		opts := []pcl.BindOption{
			pcl.Loader(args.BinderLoader),
			pcl.DirPath(args.ComponentSource),
			pcl.ComponentBinder(componentBinder),
		}
		if args.SkipResourceTypecheck {
			opts = append(opts, pcl.SkipResourceTypechecking)
		}
		if args.SkipInvokeTypecheck {
			opts = append(opts, pcl.SkipInvokeTypechecking)
		}
		return pcl.BindProgram(p.Files, opts...)
	}

	// Parse & bind the parent PCL with component support.
	p := syntax.NewParser()
	err := p.ParseFile(strings.NewReader(parentPCL), "main.pp")
	require.NoError(t, err)
	require.False(t, p.Diagnostics.HasErrors(), p.Diagnostics.Error())

	program, bindDiags, err := pcl.BindProgram(p.Files,
		pcl.Loader(loader),
		pcl.DirPath("."), // arbitrary; the in-memory binder ignores it
		pcl.ComponentBinder(componentBinder),
	)
	require.NoError(t, err)
	require.False(t, bindDiags.HasErrors(), bindDiags.Error())

	// Generate HCL (produces parent main.tf + component subdirs).
	files, genDiags, err := codegen.GenerateProgram(program)
	require.NoError(t, err)
	require.False(t, genDiags.HasErrors(), genDiags.Error())

	// Golden file snapshot
	for name, content := range files {
		goldenPath := filepath.Join("testdata", t.Name(), name)
		if cmdutil.IsTruthy(os.Getenv("PULUMI_ACCEPT")) {
			require.NoError(t, os.MkdirAll(filepath.Dir(goldenPath), 0o755))
			require.NoError(t, os.WriteFile(goldenPath, content, 0o644))
		} else {
			expected, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file %s not found; run with PULUMI_ACCEPT=1 to generate", goldenPath)
			assert.Equal(t, string(expected), string(content))
		}
	}

	// Write generated HCL to a work directory for the engine's module loader.
	outDir := t.TempDir()
	for name, content := range files {
		outPath := filepath.Join(outDir, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(outPath), 0o755))
		require.NoError(t, os.WriteFile(outPath, content, 0o644))
	}

	// Parse the generated parent HCL.
	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseDirectory(outDir)
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())

	// Run through engine.
	if mock == nil {
		mock = &testutil.MockResourceMonitor{}
	}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         outDir,
		RootDir:         outDir,
		SchemaLoader:    loader,
	})

	err = engine.Run(t.Context())
	require.NoError(t, err)

	return mock
}

// TestRequiredProvidersWithoutVersion exercises the language host with a
// `terraform { required_providers { ... } }` block whose entries omit the
// optional `version` attribute. SDK documentation snippets generated with
// codegen.SkipRequiredProvidersVersion() produce input of this shape, so
// the language host must accept it.
func TestRequiredProvidersWithoutVersion(t *testing.T) {
	t.Parallel()

	src := `terraform {
  required_providers {
    aws = {
      source = "pulumi/aws"
    }
  }
}

resource "aws_instance" "web" {
  ami           = "ami-12345"
  instance_type = "t2.micro"
}

output "instance_ami" {
  value = aws_instance.web.ami
}`

	awsSchema := schema.PackageSpec{
		Name:    "aws",
		Version: "6.0.0",
		Resources: map[string]schema.ResourceSpec{
			"aws:index:Instance": {
				InputProperties: map[string]schema.PropertySpec{
					"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
					"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}

	loader := schemaloader.New(t, awsSchema)

	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseSource("main.tf", []byte(src))
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())

	mock := &testutil.MockResourceMonitor{}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader:    loader,
	})

	require.NoError(t, engine.Run(t.Context()))

	require.Len(t, mock.RegisteredResources, 2)
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	assert.Equal(t, "aws:index:Instance", mock.RegisteredResources[1].Type)
	assert.Equal(t, "web", mock.RegisteredResources[1].Name)
}

// TestGenerateProgramSkipRequiredProvidersVersion verifies the codegen option
// omits the `version` attribute on every required_providers entry while
// preserving `source`, and that the resulting HCL still parses and executes
// end-to-end through the language host.
func TestGenerateProgramSkipRequiredProvidersVersion(t *testing.T) {
	t.Parallel()

	const pclSrc = `package "aws" {
  baseProviderName = "aws"
  baseProviderVersion = "6.0.0"
}

resource web "aws:index:Instance" {
  ami = "ami-12345"
  instanceType = "t2.micro"
}
`

	awsSchema := schema.PackageSpec{
		Name:    "aws",
		Version: "6.0.0",
		Resources: map[string]schema.ResourceSpec{
			"aws:index:Instance": {
				InputProperties: map[string]schema.PropertySpec{
					"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
					"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
				},
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"ami":          {TypeSpec: schema.TypeSpec{Type: "string"}},
						"instanceType": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
	loader := schemaloader.New(t, awsSchema)

	p := syntax.NewParser()
	require.NoError(t, p.ParseFile(strings.NewReader(pclSrc), "main.pp"))
	require.False(t, p.Diagnostics.HasErrors(), p.Diagnostics.Error())

	program, bindDiags, err := pcl.BindProgram(p.Files,
		pcl.Loader(loader),
		pcl.AllowMissingProperties,
		pcl.AllowMissingVariables,
		pcl.SkipResourceTypechecking,
	)
	require.NoError(t, err)
	require.False(t, bindDiags.HasErrors(), bindDiags.Error())

	files, diags, err := codegen.GenerateProgram(program, codegen.SkipRequiredProvidersVersion())
	require.NoError(t, err)
	require.False(t, diags.HasErrors(), diags.Error())
	require.Len(t, files, 1)

	var got []byte
	for _, content := range files {
		got = content
	}
	assert.Contains(t, string(got), `source = "pulumi/aws"`)
	assert.NotContains(t, string(got), "version =", "version should be omitted from required_providers entries")

	// Round-trip: write the generated HCL out, parse it back, and run
	// through the engine to prove it's still executable.
	outDir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(outDir, name), content, 0o600))
	}

	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseDirectory(outDir)
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())

	mock := &testutil.MockResourceMonitor{}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         outDir,
		RootDir:         outDir,
		SchemaLoader:    loader,
	})
	require.NoError(t, engine.Run(t.Context()))

	require.Len(t, mock.RegisteredResources, 2)
	assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)
	assert.Equal(t, "aws:index:Instance", mock.RegisteredResources[1].Type)
	assert.Equal(t, "web", mock.RegisteredResources[1].Name)
}
