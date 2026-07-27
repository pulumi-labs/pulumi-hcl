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

// Package configs is an in-tree shim for the one function of OpenTofu's
// internal/configs package that vendored/statefile's v3-state upgrade calls.
// regen.sh rewrites the upstream import to point here so the vendored parser
// does not drag in the full configuration loader.
package configs

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi-labs/pulumi-hcl/vendored/addrs"
	"github.com/pulumi-labs/pulumi-hcl/vendored/tfdiags"
)

// ParseProviderConfigCompactStr parses a compact provider-configuration
// address ("aws" or "aws.alias"), mirroring the upstream function of the same
// name.
func ParseProviderConfigCompactStr(str string) (addrs.LocalProviderConfig, tfdiags.Diagnostics) {
	var diags tfdiags.Diagnostics

	traversal, parseDiags := hclsyntax.ParseTraversalAbs([]byte(str), "", hcl.Pos{Line: 1, Column: 1})
	diags = diags.Append(parseDiags)
	if parseDiags.HasErrors() {
		return addrs.LocalProviderConfig{}, diags
	}

	ret := addrs.LocalProviderConfig{LocalName: traversal.RootName()}
	if len(traversal) < 2 {
		return ret, diags
	}
	if attr, ok := traversal[1].(hcl.TraverseAttr); ok && len(traversal) == 2 {
		ret.Alias = attr.Name
		return ret, diags
	}
	diags = diags.Append(&hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Invalid provider configuration address",
		Detail:   "The provider type name must either stand alone or be followed by an alias name separated with a dot.",
		Subject:  traversal[1:].SourceRange().Ptr(),
	})
	return ret, diags
}
