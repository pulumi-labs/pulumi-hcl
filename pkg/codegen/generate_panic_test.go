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

package codegen

import (
	"strings"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/syntax"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/stretchr/testify/require"
)

// TestGenerateProgramNilResourceSchema asserts that GenerateProgram does not
// panic when a *pcl.Resource referenced from a scope traversal expression has
// a nil Schema field.
//
// pulumi/pulumi-terraform-bridge invokes pcl.BindProgram with relaxed options
// (AllowMissingProperties, AllowMissingVariables, SkipResourceTypechecking)
// while converting third-party Terraform docs.  Under those options the binder
// can leave Resource.Schema unset.  scopeTraversalTokens (generate.go:2259)
// dereferences part.Schema.Properties unconditionally, which crashes with
// "invalid memory address or nil pointer dereference" on inputs the bridge
// routinely produces.
//
// Stack from a failing CI run on the bridge:
//
//	github.com/pulumi-labs/pulumi-hcl/pkg/codegen.(*generator).scopeTraversalTokens
//		/.../pkg/codegen/generate.go:2259
//	github.com/pulumi-labs/pulumi-hcl/pkg/codegen.(*generator).exprTokens
//		/.../pkg/codegen/generate.go:1748
//	github.com/pulumi-labs/pulumi-hcl/pkg/codegen.(*generator).genExpression
//		/.../pkg/codegen/generate.go:1633
//	github.com/pulumi-labs/pulumi-hcl/pkg/codegen.(*generator).genOutput
//		/.../pkg/codegen/generate.go:1535
//	github.com/pulumi-labs/pulumi-hcl/pkg/codegen.GenerateProgram
//		/.../pkg/codegen/generate.go:131
func TestGenerateProgramNilResourceSchema(t *testing.T) {
	t.Parallel()

	const src = `
resource aResource "simple:index/resource:Resource" {
    inputOne = "hello"
}

output someOutput {
    value = aResource.result
}
`
	parser := syntax.NewParser()
	require.NoError(t, parser.ParseFile(strings.NewReader(src), "main.pp"))
	require.False(t, parser.Diagnostics.HasErrors(), parser.Diagnostics.Error())

	program, diags, err := pcl.BindProgram(parser.Files,
		pcl.AllowMissingProperties,
		pcl.AllowMissingVariables,
		pcl.SkipResourceTypechecking,
	)
	require.NoError(t, err)
	require.False(t, diags.HasErrors(), diags.Error())

	require.NotPanics(t, func() {
		_, _, err := GenerateProgram(program)
		require.NoError(t, err)
	})
}
