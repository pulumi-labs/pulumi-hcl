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

package ast

import (
	"maps"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi-hcl/vendored/hcl2shim"
)

// mergedBlockTypes are the block types an override amends rather than
// replaces. Each holds arguments that are read one by one - meta-arguments,
// or the arguments a `_` block escapes from being read that way - so a
// setting the base makes has to survive an override block that does not
// mention it. Every other block type holds configuration that only makes
// sense as a whole.
var mergedBlockTypes = map[string]bool{
	"lifecycle": true,
	"pulumi":    true,
	"_":         true,
}

// MergeOverride returns the body a block declared in a primary file takes on
// once the block that overrides it in an override file is applied: arguments
// the override sets replace the originals, arguments it omits keep their
// original value, and a block type it declares hides the base blocks of that
// type (a `dynamic` block counts as the type it generates). Arguments named
// in ignored keep the base's value whatever the override says.
func MergeOverride(base, override hcl.Body, ignored ...string) hcl.Body {
	baseBody, baseNative := base.(*hclsyntax.Body)
	overrideBody, overrideNative := override.(*hclsyntax.Body)
	if !baseNative || !overrideNative {
		// JSON configuration merges through a schema instead, which leaves
		// the merged body opaque to the callers that walk the syntax tree -
		// the same way they already treat a JSON file's own body.
		return hcl2shim.MergeBodies(base, override)
	}

	merged := &hclsyntax.Body{
		Attributes: make(hclsyntax.Attributes, len(baseBody.Attributes)),
		SrcRange:   baseBody.SrcRange,
		EndRange:   baseBody.EndRange,
	}
	maps.Copy(merged.Attributes, baseBody.Attributes)
	maps.Copy(merged.Attributes, overrideBody.Attributes)
	for _, name := range ignored {
		delete(merged.Attributes, name)
		if attr, ok := baseBody.Attributes[name]; ok {
			merged.Attributes[name] = attr
		}
	}

	// Blocks the override declares, by the type they contribute.
	overridden := make(map[string]*hclsyntax.Block, len(overrideBody.Blocks))
	for _, block := range overrideBody.Blocks {
		overridden[blockType(block)] = block
	}

	emitted := make(map[string]bool, len(overridden))
	for _, block := range baseBody.Blocks {
		typeName := blockType(block)
		other, isOverridden := overridden[typeName]
		if !isOverridden {
			merged.Blocks = append(merged.Blocks, block)
			continue
		}
		if mergedBlockTypes[typeName] {
			amended := *block
			amended.Body = MergeOverride(block.Body, other.Body).(*hclsyntax.Body)
			merged.Blocks = append(merged.Blocks, &amended)
			emitted[typeName] = true
		}
	}
	for _, block := range overrideBody.Blocks {
		if !emitted[blockType(block)] {
			merged.Blocks = append(merged.Blocks, block)
		}
	}

	return merged
}

// blockType is the block type a block contributes to its body: for a dynamic
// block that is the type it generates rather than "dynamic" itself.
func blockType(block *hclsyntax.Block) string {
	if block.Type == "dynamic" && len(block.Labels) > 0 {
		return block.Labels[0]
	}
	return block.Type
}
