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

package run_test

import (
	"context"
	"fmt"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/schemaloader"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge/info"
	shim "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim"
	schemashim "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim/schema"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// singularBlockPackage describes a resource whose TF schema carries a
// MaxItems=1 block: the bridge flattens it to a single object property.
func singularBlockPackage() schema.PackageSpec {
	return schema.PackageSpec{
		Name: "simple",
		Meta: &schema.MetadataSpec{ModuleFormat: `(.*)(?:/[^/]*)`},
		Resources: map[string]schema.ResourceSpec{
			"simple:index:Resource": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Properties: map[string]schema.PropertySpec{
						"block": {TypeSpec: schema.TypeSpec{
							Ref: "#/types/simple:index:Block",
						}},
					},
					Required: []string{"block"},
				},
			},
		},
		Types: map[string]schema.ComplexTypeSpec{
			"simple:index:Block": {
				ObjectTypeSpec: schema.ObjectTypeSpec{
					Type: "object",
					Properties: map[string]schema.PropertySpec{
						"field": {TypeSpec: schema.TypeSpec{Type: "string"}},
					},
				},
			},
		},
	}
}

// singularBlockInfoSource serves the bridge mapping for the "simple" provider:
// `block` is a TypeList MaxItems=1 block, so the mapping marks it MaxItemsOne.
type singularBlockInfoSource struct{}

func (singularBlockInfoSource) GetProviderInfo(
	_ context.Context, tfProvider string, _ *workspace.PackageDescriptor,
) (*tfbridge.ProviderInfo, error) {
	if tfProvider != "simple" {
		return nil, fmt.Errorf("unknown provider %q", tfProvider)
	}
	return &tfbridge.ProviderInfo{
		Name: "simple",
		P: (&schemashim.Provider{
			ResourcesMap: schemashim.ResourceMap{
				"simple_resource": (&schemashim.Resource{Schema: schemashim.SchemaMap{
					"block": (&schemashim.Schema{
						Type:     shim.TypeList,
						Required: true,
						MaxItems: 1,
						Elem: (&schemashim.Resource{Schema: schemashim.SchemaMap{
							"field": (&schemashim.Schema{Type: shim.TypeString, Computed: true}).Shim(),
						}}).Shim(),
					}).Shim(),
				}}).Shim(),
			},
		}).Shim(),
		Resources: map[string]*info.Resource{
			"simple_resource": {Tok: "simple:index:Resource"},
		},
	}, nil
}

// TestEngine_SingularBlockUnknownDuringPreview indexes into a MaxItems=1 block
// (`block[0].field`) whose value the monitor omits from the preview outputs,
// the way the engine drops unknown properties. The unknown placeholder is
// typed from the flattened Pulumi object, so it must be re-wrapped as a list
// for the index to resolve.
func TestEngine_SingularBlockUnknownDuringPreview(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "simple_resource" "r" {
}

output "field" {
  value = simple_resource.r.block[0].field
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	require.Empty(t, diags)

	mock := &testutil.MockResourceMonitor{
		DryRun: true,
		RegisterResourceHandler: func(_ context.Context, req run.RegisterResourceRequest) (*run.RegisterResourceResponse, error) {
			return &run.RegisterResourceResponse{
				URN: urn.URN("urn:pulumi:test::project::" + req.Type + "::" + req.Name),
				// No outputs: unknown properties are dropped during preview.
				Outputs: property.Map{},
			}, nil
		},
	}
	engine := newTestEngine(t, config, &run.EngineOptions{
		ModuleLoader:       testModuleLoader(t),
		ProjectName:        "test-project",
		StackName:          "dev",
		ResourceMonitor:    mock,
		WorkDir:            t.TempDir(),
		RootDir:            t.TempDir(),
		DryRun:             true,
		SchemaLoader:       schemaloader.New(t, singularBlockPackage()),
		ProviderInfoSource: singularBlockInfoSource{},
	})
	require.NoError(t, engine.Run(t.Context()))

	assert.True(t, mock.StackOutputs.Get("field").IsComputed(),
		"the traversal through the unknown block should stay unknown during preview")
}
