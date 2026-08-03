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
	"testing"

	"github.com/pulumi/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi-hcl/tests/testutil"
	"github.com/pulumi/pulumi-hcl/tests/testutil/schemaloader"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_PulumiOptionsNestedPaths checks that `hide_diffs` and
// `replace_on_changes` entries in the `pulumi` block may be multi-segment
// property paths, matching `lifecycle.ignore_changes` (which accepts the same
// shapes), rather than being treated as references to other nodes.
func TestEngine_PulumiOptionsNestedPaths(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "pfx_res" "res" {
  tags = {
    stage = "one"
    owner = "team"
  }

  pulumi {
    hide_diffs         = [tags["stage"]]
    replace_on_changes = [tags["owner"]]
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

	tagsProp := schema.PropertySpec{TypeSpec: schema.TypeSpec{
		Type:                 "object",
		AdditionalProperties: &schema.TypeSpec{Type: "string"},
	}}
	mock := &testutil.MockResourceMonitor{}
	engine := newTestEngine(t, config, &run.EngineOptions{
		ModuleLoader:    testModuleLoader(t),
		ProjectName:     "test-project",
		StackName:       "dev",
		ResourceMonitor: mock,
		WorkDir:         t.TempDir(),
		RootDir:         t.TempDir(),
		SchemaLoader: schemaloader.New(t, schema.PackageSpec{
			Name: "pfx",
			Resources: map[string]schema.ResourceSpec{
				"pfx:index:Res": {
					InputProperties: map[string]schema.PropertySpec{"tags": tagsProp},
					ObjectTypeSpec: schema.ObjectTypeSpec{
						Properties: map[string]schema.PropertySpec{"tags": tagsProp},
					},
				},
			},
		}),
	})

	require.NoError(t, engine.Run(t.Context()))

	require.Len(t, mock.RegisteredResources, 2)
	req := mock.RegisteredResources[1]
	assert.Equal(t, []property.Glob{
		property.GlobFromSegments(property.NewSegment("tags"), property.NewSegment("stage")),
	}, req.HideDiffs)
	assert.Equal(t, []property.Glob{
		property.GlobFromSegments(property.NewSegment("tags"), property.NewSegment("owner")),
	}, req.ReplaceOnChanges)
}
