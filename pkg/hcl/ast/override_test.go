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
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// parseBody parses src as the body of a block written in native syntax.
func parseBody(t *testing.T, src string) hcl.Body {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.Pos{Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), diags.Error())
	return file.Body
}

// bodySummary describes a merged body as arguments (evaluated) plus the
// blocks it holds, keyed by type, so a whole merge can be asserted at once.
func bodySummary(t *testing.T, body hcl.Body) map[string]any {
	t.Helper()
	native, ok := body.(*hclsyntax.Body)
	require.True(t, ok, "expected a native body")

	out := map[string]any{}
	for name, attr := range native.Attributes {
		value, diags := attr.Expr.Value(nil)
		require.False(t, diags.HasErrors(), diags.Error())
		out[name] = value
	}
	for _, block := range native.Blocks {
		key := block.Type
		if len(block.Labels) > 0 {
			key += " " + block.Labels[0]
		}
		nested := bodySummary(t, block.Body)
		if existing, seen := out[key]; seen {
			out[key] = append(existing.([]map[string]any), nested)
			continue
		}
		out[key] = []map[string]any{nested}
	}
	return out
}

func TestMergeOverrideArgumentsAndBlocks(t *testing.T) {
	t.Parallel()

	base := parseBody(t, `
kept      = "base"
replaced  = "base"
nested { value = "base" }
other { value = "base" }
`)
	override := parseBody(t, `
replaced = "override"
added    = "override"
nested { value = "override" }
`)

	assert.Equal(t, map[string]any{
		"kept":     cty.StringVal("base"),
		"replaced": cty.StringVal("override"),
		"added":    cty.StringVal("override"),
		// The override declares `nested`, so it hides the base's `nested`
		// block, while `other` is untouched.
		"nested": []map[string]any{{"value": cty.StringVal("override")}},
		"other":  []map[string]any{{"value": cty.StringVal("base")}},
	}, bodySummary(t, MergeOverride(base, override)))
}

func TestMergeOverrideDynamicBlocks(t *testing.T) {
	t.Parallel()

	// A `dynamic` block counts as the type it generates: declaring `nested`
	// statically in the override hides the base's `dynamic "nested"`, and the
	// base's unrelated dynamic block survives.
	base := parseBody(t, `
dynamic "nested" {
  content {
    value = "base"
  }
}
dynamic "other" {
  content {
    value = "base"
  }
}
`)
	override := parseBody(t, `nested { value = "override" }`)

	assert.Equal(t, map[string]any{
		"nested": []map[string]any{{"value": cty.StringVal("override")}},
		"dynamic other": []map[string]any{{
			"content": []map[string]any{{"value": cty.StringVal("base")}},
		}},
	}, bodySummary(t, MergeOverride(base, override)))
}

func TestMergeOverrideMergedBlockTypes(t *testing.T) {
	t.Parallel()

	// lifecycle, pulumi and `_` blocks hold arguments that are read one by
	// one, so an override block amends them instead of replacing them: the
	// base's settings - including its precondition - survive.
	base := parseBody(t, `
lifecycle {
  ignore_changes  = ["input_one"]
  prevent_destroy = true
  precondition {
    condition     = true
    error_message = "gate is closed"
  }
}
_ { input = "base" }
`)
	override := parseBody(t, `
lifecycle { prevent_destroy = false }
_ { triggers_replace = "override" }
`)

	assert.Equal(t, map[string]any{
		"lifecycle": []map[string]any{{
			"ignore_changes":  cty.TupleVal([]cty.Value{cty.StringVal("input_one")}),
			"prevent_destroy": cty.False,
			"precondition": []map[string]any{{
				"condition":     cty.True,
				"error_message": cty.StringVal("gate is closed"),
			}},
		}},
		"_": []map[string]any{{
			"input":            cty.StringVal("base"),
			"triggers_replace": cty.StringVal("override"),
		}},
	}, bodySummary(t, MergeOverride(base, override)))
}

func TestMergeOverrideIgnoredArguments(t *testing.T) {
	t.Parallel()

	t.Run("base keeps its own", func(t *testing.T) {
		t.Parallel()
		merged := MergeOverride(
			parseBody(t, `for_each = "base"`+"\n"+`prefix = "base"`),
			parseBody(t, `for_each = "override"`+"\n"+`prefix = "override"`),
			"for_each")
		assert.Equal(t, map[string]any{
			"for_each": cty.StringVal("base"),
			"prefix":   cty.StringVal("override"),
		}, bodySummary(t, merged))
	})

	t.Run("override cannot introduce one", func(t *testing.T) {
		t.Parallel()
		merged := MergeOverride(
			parseBody(t, `prefix = "base"`),
			parseBody(t, `for_each = "override"`),
			"for_each")
		assert.Equal(t, map[string]any{"prefix": cty.StringVal("base")}, bodySummary(t, merged))
	})
}

func TestMergeOverrideJSONBody(t *testing.T) {
	t.Parallel()

	// JSON configuration has no syntax tree to merge into, so it falls back
	// to a schema-driven merge. The override semantics still hold.
	base, diags := json.Parse([]byte(`{"kept": "base", "replaced": "base"}`), "test.tf.json")
	require.False(t, diags.HasErrors(), diags.Error())
	override, diags := json.Parse([]byte(`{"replaced": "override"}`), "override.tf.json")
	require.False(t, diags.HasErrors(), diags.Error())

	merged := MergeOverride(base.Body, override.Body)
	content, _, contentDiags := merged.PartialContent(&hcl.BodySchema{
		Attributes: []hcl.AttributeSchema{{Name: "kept"}, {Name: "replaced"}},
	})
	require.False(t, contentDiags.HasErrors(), contentDiags.Error())

	values := map[string]cty.Value{}
	for name, attr := range content.Attributes {
		value, valDiags := attr.Expr.Value(nil)
		require.False(t, valDiags.HasErrors(), valDiags.Error())
		values[name] = value
	}
	assert.Equal(t, map[string]cty.Value{
		"kept":     cty.StringVal("base"),
		"replaced": cty.StringVal("override"),
	}, values)
}
