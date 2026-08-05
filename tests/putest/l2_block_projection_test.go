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

package putest_test

import (
	"testing"

	"github.com/pulumi/pulumi-hcl/tests/testutil/putest"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfexec"
	"github.com/stretchr/testify/assert"
)

// These cases pin the correct (OpenTofu-matching) behavior of the linked-in
// bridged provider path. Each mirrors a tfcompat case of the same name that
// is skipped as a known failure because the terraform-provider plugin path
// diverges (its wire-schema projection drops empty nested collections and
// set typing). When those divergences are fixed, re-enable the tfcompat
// twins and delete these.

// TestL2OmittedNestedBlock: an optional MaxItems=1 nested block (`rule`) left
// out of an enclosing block reads as an empty list of blocks (`[]`, length 0).
func TestL2OmittedNestedBlock(t *testing.T) {
	t.Parallel()
	putest.RunCase(t, "l2_omitted_nested_block", putest.Case{
		Providers: []putest.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
		ExpectedOutputs: map[string]string{
			"rule":     "[]",
			"rule_len": "0",
		},
	})
}

// TestL2OptionalComputedBlockUnset: an Optional+Computed MaxItems=1 block the
// provider leaves unset reads as null for the TypeList variant and as the
// empty set for the TypeSet variant.
func TestL2OptionalComputedBlockUnset(t *testing.T) {
	t.Parallel()
	putest.RunCase(t, "l2_optional_computed_block_unset", putest.Case{
		Providers: []putest.Provider{
			{Name: "optcomp", Factory: providers.OptCompBlockProvider},
		},
		ExpectedOutputs: map[string]string{
			"identity_is_null":     "true",
			"identity_json":        "null",
			"identity_set_is_null": "false",
			"identity_set_json":    "[]",
		},
	})
}

// TestL2SetBlockEquality: a repeating TypeSet nested block materializes as a
// cty set, so comparing it to a `toset(...)` of the same elements is true
// regardless of element order.
func TestL2SetBlockEquality(t *testing.T) {
	t.Parallel()
	putest.RunCase(t, "l2_set_block_equality", putest.Case{
		Providers: []putest.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
		ExpectedOutputs: map[string]string{
			"eq_same":      "true",
			"eq_reordered": "true",
		},
	})
}

// TestL2IgnoreChangesForceNewBlockRemoved: removing a MaxItems=1 block whose
// ForceNew `mode` is in ignore_changes replaces the resource (the ignored
// index no longer exists, so the ForceNew change is observed), and `settings`
// reports the empty list the replacement was created with.
func TestL2IgnoreChangesForceNewBlockRemoved(t *testing.T) {
	t.Parallel()
	putest.RunCase(t, "l2_ignore_changes_forcenew_block_removed", putest.Case{
		Providers: []putest.Provider{
			{Name: "fnblock", Factory: providers.FNBlockProvider},
		},
		ExpectedOutputs: map[string]string{
			"settings": "[]",
		},
		AssertOps: func(t *testing.T, ops []tfexec.Op) {
			oldSettings := []any{map[string]any{"mode": "a", "verbose": false}}
			assert.Equal(t, []tfexec.Op{
				{
					Kind:    tfexec.OpCreate,
					Type:    "fnblock_resource",
					Inputs:  map[string]any{"note": "x", "settings": oldSettings},
					Outputs: map[string]any{"note": "x", "settings": oldSettings},
				},
				{
					Kind:    tfexec.OpCreate,
					Type:    "fnblock_resource",
					Inputs:  map[string]any{"note": "y", "settings": []any{}},
					Outputs: map[string]any{"note": "y", "settings": []any{}},
				},
				{
					Kind:   tfexec.OpDelete,
					Type:   "fnblock_resource",
					Inputs: map[string]any{"note": "x", "settings": oldSettings},
				},
			}, ops)
		},
	})
}
