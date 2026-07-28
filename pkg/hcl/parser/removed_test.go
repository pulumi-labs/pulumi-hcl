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

package parser

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
)

func TestParseRemovedBlock(t *testing.T) {
	t.Parallel()
	src := []byte(`
removed {
  from = simple_resource.a

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}

removed {
  from = module.gone

  lifecycle {
    destroy = true
  }
}
`)

	config, diags := NewParser().ParseSource("main.tf", src)
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	require.Len(t, config.Removed, 2)

	assert.Equal(t, ast.TargetAddr{Type: "simple_resource", Name: "a"}, config.Removed[0].From)
	assert.True(t, config.Removed[0].Destroy)
	require.Len(t, config.Removed[0].Provisioners, 1)
	assert.Equal(t, "local-exec", config.Removed[0].Provisioners[0].Type)
	assert.Equal(t, "destroy", config.Removed[0].Provisioners[0].When)

	assert.Equal(t, ast.TargetAddr{Module: modulepath.Root().Append(modulepath.NewStep("gone"))}, config.Removed[1].From)
	assert.True(t, config.Removed[1].Destroy)
	assert.Empty(t, config.Removed[1].Provisioners)
}

// The destroy argument converts like any other decoded attribute, so a
// string boolean is accepted.
func TestParseRemovedBlockDestroyStringBool(t *testing.T) {
	t.Parallel()
	src := []byte(`
removed {
  from = simple_resource.a

  lifecycle {
    destroy = "true"
  }
}
`)

	config, diags := NewParser().ParseSource("main.tf", src)
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	require.Len(t, config.Removed, 1)
	assert.True(t, config.Removed[0].Destroy)
}

// A provisioner-carrying removed block may target a resource inside a module.
func TestParseRemovedBlockModulePrefixedProvisioner(t *testing.T) {
	t.Parallel()
	src := []byte(`
removed {
  from = module.child.simple_resource.a

  lifecycle {
    destroy = true
  }

  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}
`)

	config, diags := NewParser().ParseSource("main.tf", src)
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	require.Len(t, config.Removed, 1)
	assert.Equal(t, ast.TargetAddr{
		Module: modulepath.Root().Append(modulepath.NewStep("child")),
		Type:   "simple_resource",
		Name:   "a",
	}, config.Removed[0].From)
	require.Len(t, config.Removed[0].Provisioners, 1)
}

// destroy = false is unsupported but still parses: the block lands in the AST
// with Destroy false and the failure is an error diagnostic, not a dropped
// block.
func TestParseRemovedBlockDestroyFalse(t *testing.T) {
	t.Parallel()
	src := []byte(`
removed {
  from = simple_resource.a

  lifecycle {
    destroy = false
  }
}
`)

	config, diags := NewParser().ParseSource("main.tf", src)
	require.Len(t, diags, 1)
	assert.Equal(t, "Unsupported removed block", diags[0].Summary)
	require.Len(t, config.Removed, 1)
	assert.False(t, config.Removed[0].Destroy)
}

func TestParseRemovedBlockErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name        string
		src         string
		wantSummary string
	}{
		{
			name: "destroy false",
			src: `
removed {
  from = simple_resource.a
  lifecycle {
    destroy = false
  }
}`,
			wantSummary: "Unsupported removed block",
		},
		{
			name: "lifecycle omitted",
			src: `
removed {
  from = simple_resource.a
}`,
			wantSummary: "Unsupported removed block",
		},
		{
			name: "instance key",
			src: `
removed {
  from = simple_resource.a[0]
  lifecycle {
    destroy = true
  }
}`,
			wantSummary: "Resource instance keys not allowed",
		},
		{
			name: "data source address",
			src: `
removed {
  from = data.simple_data.a
  lifecycle {
    destroy = true
  }
}`,
			wantSummary: "Invalid \"from\" address",
		},
		{
			name: "bare name address",
			src: `
removed {
  from = foo
  lifecycle {
    destroy = true
  }
}`,
			wantSummary: "Invalid \"from\" address",
		},
		{
			name: "too many address parts",
			src: `
removed {
  from = simple_resource.a.b
  lifecycle {
    destroy = true
  }
}`,
			wantSummary: "Invalid \"from\" address",
		},
		{
			name: "incomplete module-prefixed address",
			src: `
removed {
  from = module.child.simple_resource
  lifecycle {
    destroy = true
  }
}`,
			wantSummary: "Invalid \"from\" address",
		},
		{
			name: "create provisioner",
			src: `
removed {
  from = simple_resource.a
  lifecycle {
    destroy = true
  }
  provisioner "local-exec" {
    command = "true"
  }
}`,
			wantSummary: "Invalid \"removed.provisioner\" block",
		},
		{
			name: "module address with provisioner",
			src: `
removed {
  from = module.gone
  lifecycle {
    destroy = true
  }
  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}`,
			wantSummary: "Invalid removed block",
		},
		{
			name: "nested module address with provisioner",
			src: `
removed {
  from = module.gone.module.deeper
  lifecycle {
    destroy = true
  }
  provisioner "local-exec" {
    when    = destroy
    command = "true"
  }
}`,
			wantSummary: "Invalid removed block",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, diags := NewParser().ParseSource("main.tf", []byte(tt.src))
			require.Len(t, diags, 1)
			assert.Equal(t, tt.wantSummary, diags[0].Summary)
		})
	}
}

// Duplicate removed blocks without provisioners are legal, matching OpenTofu.
func TestParseRemovedBlockDuplicateWithoutProvisioners(t *testing.T) {
	t.Parallel()
	src := []byte(`
removed {
  from = simple_resource.a
  lifecycle {
    destroy = true
  }
}

removed {
  from = simple_resource.a
  lifecycle {
    destroy = true
  }
}
`)

	config, diags := NewParser().ParseSource("main.tf", src)
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	assert.Len(t, config.Removed, 2)
}

func TestRemovedBlockInOverrideFileRejected(t *testing.T) {
	t.Parallel()

	_, diags := parseDir(t, map[string]string{
		"main.tf": `resource "simple_resource" "r" { input_one = "base" }`,
		"override.tf": `
removed {
  from = simple_resource.gone
  lifecycle {
    destroy = true
  }
}`,
	})

	require.Len(t, diags, 1)
	assert.Equal(t, `Cannot override "removed" blocks`, diags[0].Summary)
}
