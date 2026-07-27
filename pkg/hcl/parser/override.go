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
	"fmt"
	"maps"
	"path/filepath"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
)

// isOverrideFile reports whether path names an override file, whose blocks
// amend blocks of the same name declared in the directory's other files
// instead of declaring new ones.
func isOverrideFile(path string) bool {
	base := filepath.Base(path)
	for _, ext := range []string{".tf.json", ".tf"} {
		if trimmed, ok := strings.CutSuffix(base, ext); ok {
			base = trimmed
			break
		}
	}
	return base == "override" || strings.HasSuffix(base, "_override")
}

// applyOverrides folds the blocks of a directory's override files into the
// blocks they override, returning the blocks to parse. Blocks whose merge can
// only happen once the primary blocks are parsed - `locals` and `terraform`,
// neither of which is addressable by block labels - are returned separately
// for mergeParsedOverride.
func applyOverrides(primary, override []*hcl.Block) (blocks, deferred []*hcl.Block, diags hcl.Diagnostics) {
	if len(override) == 0 {
		return primary, nil, nil
	}

	blocks = slices.Clone(primary)
	index := make(map[string]int, len(blocks))
	for i, block := range blocks {
		if key, ok := declKey(block); ok {
			// A duplicate declaration is an error the parser reports on its
			// own; overriding the last one keeps this from compounding it.
			index[key] = i
		}
	}

	for _, block := range override {
		switch block.Type {
		case "locals", "terraform":
			deferred = append(deferred, block)
			continue
		case "moved", "import", "removed":
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  fmt.Sprintf("Cannot override %q blocks", block.Type),
				Detail: fmt.Sprintf(
					"%s blocks can appear only in normal files, not in override files.",
					strings.ToUpper(block.Type[:1])+block.Type[1:]),
				Subject: &block.DefRange,
			})
			continue
		}

		if d := overriddenDependsOn(block); d != nil {
			diags = append(diags, d)
		}

		key, ok := declKey(block)
		if !ok {
			continue
		}
		i, exists := index[key]
		if !exists {
			// A default (unaliased) provider configuration may be introduced
			// by an override file: an absent provider configuration is
			// equivalent to an empty one, so there is nothing to typo.
			if block.Type == "provider" && providerAlias(block) == "" {
				index[key] = len(blocks)
				blocks = append(blocks, block)
				continue
			}
			diags = append(diags, missingBaseDecl(block))
			continue
		}

		merged := *blocks[i]
		merged.Body = ast.MergeOverride(blocks[i].Body, block.Body, unoverridableArgs[block.Type]...)
		blocks[i] = &merged
	}

	return blocks, deferred, diags
}

// unoverridableArgs are arguments an override file cannot change, because
// they decide how many instances the declaration has rather than how one is
// configured. A provider configuration's `for_each` expands the declaration
// the resources referencing it are written against, so an override that
// changed it would invalidate those references.
var unoverridableArgs = map[string][]string{
	"provider": {"for_each"},
}

// declKey identifies the declaration a block makes, so that a block in an
// override file can be paired with the block it overrides. It returns false
// for block types that do not declare a single addressable object.
func declKey(block *hcl.Block) (string, bool) {
	switch block.Type {
	case "resource", "data", "variable", "output", "module", "check", "call":
		return block.Type + "." + strings.Join(block.Labels, "."), true
	case "provider":
		key := "provider." + block.Labels[0]
		if alias := providerAlias(block); alias != "" {
			key += "." + alias
		}
		return key, true
	default:
		return "", false
	}
}

// providerAlias returns a provider block's alias, or "" for the default
// configuration. Malformed aliases are reported when the block is parsed.
func providerAlias(block *hcl.Block) string {
	content, _, _ := block.Body.PartialContent(providerSchema)
	attr, ok := content.Attributes["alias"]
	if !ok {
		return ""
	}
	var alias string
	if gohcl.DecodeExpression(attr.Expr, nil, &alias).HasErrors() {
		return ""
	}
	return alias
}

// dependsOnSchema picks depends_on out of a block of any type.
var dependsOnSchema = &hcl.BodySchema{
	Attributes: []hcl.AttributeSchema{{Name: "depends_on"}},
}

// overriddenDependsOn rejects a depends_on argument in an override file.
// Dependencies are not merged, so honoring the override would silently drop
// the base declaration's dependencies.
func overriddenDependsOn(block *hcl.Block) *hcl.Diagnostic {
	content, _, _ := block.Body.PartialContent(dependsOnSchema)
	attr, ok := content.Attributes["depends_on"]
	if !ok {
		return nil
	}
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  "Unsupported override",
		Detail:   "The depends_on argument may not be overridden.",
		Subject:  attr.Range.Ptr(),
	}
}

// declNouns names the declaration each overridable block type makes.
var declNouns = map[string]string{
	"resource": "resource",
	"data":     "data source",
	"variable": "variable",
	"output":   "output",
	"module":   "module call",
	"check":    "check block",
	"call":     "call block",
	"provider": "provider configuration",
}

// missingBaseDecl reports an override block that has nothing to override.
func missingBaseDecl(block *hcl.Block) *hcl.Diagnostic {
	noun, name := declNouns[block.Type], strings.Join(block.Labels, ".")
	if alias := providerAlias(block); block.Type == "provider" && alias != "" {
		name += "." + alias
	}
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("Missing %s to override", noun),
		Detail: fmt.Sprintf("There is no %s %q. An override file can only override a %s "+
			"that a primary configuration file declares.", noun, name, noun),
		Subject: &block.DefRange,
	}
}

// mergeParsedOverride applies an override block that addresses declarations
// rather than a single labeled block, once the primary files are parsed.
func (p *Parser) mergeParsedOverride(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	switch block.Type {
	case "locals":
		return mergeOverrideLocals(config, block)
	case "terraform":
		return p.mergeOverrideTerraform(config, block)
	default:
		return nil
	}
}

// mergeOverrideLocals replaces the definition of each local value the
// override block redefines. A local value is a single expression, so the
// override replaces it whole, source range included.
func mergeOverrideLocals(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	attrs, diags := block.Body.JustAttributes()

	for _, name := range slices.Sorted(maps.Keys(attrs)) {
		attr := attrs[name]
		existing, ok := config.Locals[name]
		if !ok {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Missing local value to override",
				Detail: fmt.Sprintf("There is no local value %q. An override file can only "+
					"override a local value that a primary configuration file declares.", name),
				Subject: &attr.Range,
			})
			continue
		}
		existing.Value = attr.Expr
		existing.DeclRange = attr.Range
	}

	return diags
}

// mergeOverrideTerraform folds an override file's terraform block into the
// one the primary files declare: each setting it makes replaces the original,
// and required_providers entries are replaced per provider.
func (p *Parser) mergeOverrideTerraform(config *ast.Config, block *hcl.Block) hcl.Diagnostics {
	parsed := ast.NewConfig()
	diags := p.parseTerraformBlock(parsed, block)
	if parsed.Terraform == nil {
		return diags
	}

	if config.Terraform == nil {
		config.Terraform = parsed.Terraform
		return diags
	}

	tf, override := config.Terraform, parsed.Terraform
	maps.Copy(tf.RequiredProviders, override.RequiredProviders)
	if override.RequiredVersionRange != nil {
		tf.RequiredVersionRange = override.RequiredVersionRange
	}
	if override.Component != nil {
		tf.Component = override.Component
	}
	if override.Package != nil {
		tf.Package = override.Package
	}

	return diags
}
