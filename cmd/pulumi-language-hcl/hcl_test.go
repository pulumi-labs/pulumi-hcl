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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/codegen"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	hclrun "github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil"
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

func TestNotImplemented(t *testing.T) {
	t.Parallel()

	generateHCL := func(t *testing.T, pclSource string) string {
		t.Helper()

		loader := testutil.NewMockReferenceLoader(t)

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

		generatedHCL := string(files["main.hcl"])
		require.NotEmpty(t, generatedHCL)

		hclParser := parser.NewParser()
		_, hclDiags := hclParser.ParseSource("main.hcl", files["main.hcl"])
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

func testConvertedPCL(t *testing.T, pclSource string, schemas ...schema.PackageSpec) *testutil.MockResourceMonitor {
	t.Helper()
	return testConvertedPCLWithComponent(t, pclSource, nil, nil, schemas...)
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

	loader := testutil.NewMockReferenceLoader(t, awsSchema)

	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseSource("main.hcl", []byte(src))
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

	loader := testutil.NewMockReferenceLoader(t, schemas...)

	// Build an in-memory ComponentProgramBinder so we don't need files on disk
	// for the PCL binding step.
	componentBinder := func(args pcl.ComponentProgramBinderArgs) (*pcl.Program, hcl.Diagnostics, error) {
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
		opts := []pcl.BindOption{pcl.Loader(args.BinderLoader)}
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

	// Generate HCL (produces parent main.hcl + component subdirs).
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
