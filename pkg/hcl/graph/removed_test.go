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

package graph

import (
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// A removed block targeting a resource inside a child module errors during
// inlining when the child still declares that resource, and builds cleanly
// once the resource is gone.
func TestRemovedResourceInModule(t *testing.T) {
	t.Parallel()

	rootSrc := []byte(`
module "m" {
  source = "./child"
}

removed {
  from = module.m.simple_resource.a
  lifecycle {
    destroy = true
  }
}
`)
	rootConfig, diags := parser.NewParser().ParseSource("root.hcl", rootSrc)
	require.False(t, diags.HasErrors(), diags.Error())

	stillExists, diags := parser.NewParser().ParseSource("child.hcl", []byte(`
resource "simple_resource" "a" {}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	_, err := BuildFromConfig(rootConfig, fakeModuleLoader{modules: map[string]*LoadedModule{
		"./child": {Config: stillExists, SourcePath: "./child"},
	}}, "")
	assert.EqualError(t, err,
		"inlining module m: removed block for module.m.simple_resource.a: this resource block still exists in the configuration")

	gone, diags := parser.NewParser().ParseSource("child.hcl", []byte(`
resource "simple_resource" "b" {}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	_, err = BuildFromConfig(rootConfig, fakeModuleLoader{modules: map[string]*LoadedModule{
		"./child": {Config: gone, SourcePath: "./child"},
	}}, "")
	assert.NoError(t, err)
}

// A removed block targeting a nested module errors when that module is still
// declared; the depth-one case is already rejected at parse time.
func TestRemovedNestedModuleStillExists(t *testing.T) {
	t.Parallel()

	rootSrc := []byte(`
module "m" {
  source = "./child"
}

removed {
  from = module.m.module.n
  lifecycle {
    destroy = true
  }
}
`)
	rootConfig, diags := parser.NewParser().ParseSource("root.hcl", rootSrc)
	require.False(t, diags.HasErrors(), diags.Error())

	childConfig, diags := parser.NewParser().ParseSource("child.hcl", []byte(`
module "n" {
  source = "./grand"
}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	grandConfig, diags := parser.NewParser().ParseSource("grand.hcl", []byte(`
resource "simple_resource" "a" {}
`))
	require.False(t, diags.HasErrors(), diags.Error())

	_, err := BuildFromConfig(rootConfig, fakeModuleLoader{modules: map[string]*LoadedModule{
		"./child": {Config: childConfig, SourcePath: "./child"},
		"./grand": {Config: grandConfig, SourcePath: "./grand"},
	}}, "")
	assert.EqualError(t, err,
		"inlining module m: inlining nested module n: removed block for module.m.module.n: this module block still exists in the configuration")
}
