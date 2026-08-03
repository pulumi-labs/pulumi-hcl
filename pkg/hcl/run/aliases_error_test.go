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
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestEngine_AliasesEvaluationError documents that an invalid `aliases`
// expression must fail the run instead of being silently dropped. Today the
// engine discards the error from evaluating `aliases` (run.go, "Handle
// aliases attribute"), so a non-list value deploys as if no aliases were
// set — and a rename intended to be covered by that alias would delete and
// recreate the resource.
func TestEngine_AliasesEvaluationError(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "pfx_res" "res" {
  pulumi {
    aliases = "old-name"
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

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
				"pfx:index:Res": {},
			},
		}),
	})

	err := engine.Run(t.Context())
	assert.ErrorContains(t, err, "aliases must be a list")
}

// TestEngine_AliasesParentURN pins that an alias's parent_urn can reference
// another resource's URN via pulumiresourceurn.
func TestEngine_AliasesParentURN(t *testing.T) {
	t.Parallel()

	src := []byte(`
resource "pfx_res" "first" {
}

resource "pfx_res" "second" {
  pulumi {
    aliases = [{
      parent_urn = pulumiresourceurn(pfx_res.first)
    }]
  }
}
`)

	p := parser.NewParser()
	config, diags := p.ParseSource("test.hcl", src)
	require.False(t, diags.HasErrors(), "parse error: %s", diags.Error())

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
				"pfx:index:Res": {},
			},
		}),
	})

	require.NoError(t, engine.Run(t.Context()))

	var second *run.RegisterResourceRequest
	for i := range mock.RegisteredResources {
		if mock.RegisteredResources[i].Name == "second" {
			second = &mock.RegisteredResources[i]
			break
		}
	}
	require.NotNil(t, second, "expected resource 'second' to be registered")

	assert.Equal(t, []run.Alias{{Spec: &run.AliasSpec{
		ParentURN: "urn:pulumi:test::project::pfx:index:Res::first",
	}}}, second.Aliases)
}
