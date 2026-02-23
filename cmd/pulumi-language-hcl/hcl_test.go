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
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi/pulumi-language-hcl/pkg/codegen"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	hclrun "github.com/pulumi/pulumi-language-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi-language-hcl/tests/testutil"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestHCL(t *testing.T) {
	t.Parallel()

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

		mock := testHCL(t, pclSource, testSchema)

		require.Len(t, mock.RegisteredResources, 2, "expected stack + server")

		assert.Equal(t, "pulumi:pulumi:Stack", mock.RegisteredResources[0].Type)

		server := mock.RegisteredResources[1]
		assert.Equal(t, "test:index:Server", server.Type)
		assert.Equal(t, "myServer", server.Name)
		assert.Equal(t, resource.NewStringProperty("my-server"), server.Inputs["name"])
		assert.Equal(t, resource.NewArrayProperty([]resource.PropertyValue{
			resource.NewObjectProperty(resource.PropertyMap{
				"protocol": resource.NewStringProperty("tcp"),
				"port":     resource.NewNumberProperty(443),
			}),
			resource.NewObjectProperty(resource.PropertyMap{
				"protocol": resource.NewStringProperty("udp"),
				"port":     resource.NewNumberProperty(53),
			}),
		}), server.Inputs["networkRules"])
	})
}

func testHCL(t *testing.T, pclSource string, schemas ...schema.PackageSpec) *testutil.MockResourceMonitor {
	t.Helper()

	loader := testutil.NewMockReferenceLoader(t, schemas...)

	// Parse PCL
	p := syntax.NewParser()
	err := p.ParseFile(strings.NewReader(pclSource), "main.pp")
	require.NoError(t, err)
	require.False(t, p.Diagnostics.HasErrors(), p.Diagnostics.Error())

	// Bind PCL
	program, bindDiags, err := pcl.BindProgram(p.Files, pcl.Loader(loader))
	require.NoError(t, err)
	require.False(t, bindDiags.HasErrors(), bindDiags.Error())

	// Generate HCL
	files, genDiags, err := codegen.GenerateProgram(program)
	require.NoError(t, err)
	require.False(t, genDiags.HasErrors(), genDiags.Error())

	generatedHCL := files["main.hcl"]
	require.NotEmpty(t, generatedHCL, "expected generated HCL output")

	// Golden file snapshot
	goldenPath := filepath.Join("testdata", t.Name(), "main.hcl")
	if cmdutil.IsTruthy(os.Getenv("PULUMI_ACCEPT")) {
		err := os.MkdirAll(filepath.Dir(goldenPath), 0o755)
		require.NoError(t, err)
		err = os.WriteFile(goldenPath, generatedHCL, 0o644)
		require.NoError(t, err)
	} else {
		expected, err := os.ReadFile(goldenPath)
		require.NoError(t, err, "golden file not found; run with PULUMI_ACCEPT=1 to generate")
		assert.Equal(t, string(expected), string(generatedHCL))
	}

	// Parse generated HCL
	hclParser := parser.NewParser()
	config, hclDiags := hclParser.ParseSource("main.hcl", generatedHCL)
	require.False(t, hclDiags.HasErrors(), hclDiags.Error())

	// Run through engine
	mock := &testutil.MockResourceMonitor{}
	engine := hclrun.NewEngine(config, &hclrun.EngineOptions{
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader:    loader,
	})

	err = engine.Run(t.Context())
	require.NoError(t, err)

	return mock
}
