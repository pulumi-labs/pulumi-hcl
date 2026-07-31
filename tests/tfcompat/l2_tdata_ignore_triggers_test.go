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

package tfcompat_test

import (
	"testing"

	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2TdataIgnoreTriggers covers `ignore_changes = [triggers_replace]` on a
// terraform_data resource. OpenTofu honors the ignore: a later change to
// triggers_replace produces no diff and no replacement, so the terraform_data
// id stays stable and a replace_triggered_by dependent is not touched (1 op
// total across both stages). pulumi-hcl routes triggers_replace to the engine's
// ReplacementTrigger before ignore_changes is applied, so the change still
// forces a replacement and the dependent is re-created + deleted (3 ops).
func TestL2TdataIgnoreTriggers(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_triggers", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}

// TestL2TdataIgnoreAll is the ignore_changes = all sibling of
// TestL2TdataIgnoreTriggers: the splat entry covers triggers_replace too, so a
// later change to it is ignored and the dependent is not replaced.
func TestL2TdataIgnoreAll(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_all", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}

// TestL2TdataIgnoreTriggersIndex ignores a single key of a map-valued
// triggers_replace (triggers_replace["drop"]). triggers_replace is one dynamic
// attribute, so the traversal ignores the whole attribute and even a change to
// an un-named key is suppressed, leaving the dependent untouched.
func TestL2TdataIgnoreTriggersIndex(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_triggers_index", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}

// TestL2TdataIgnoreAllInput checks that ignore_changes = all suppresses a later
// input change as well as triggers_replace: the retained input still flows to
// output and no replacement occurs.
func TestL2TdataIgnoreAllInput(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_all_input", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}

// TestL2TdataIgnoreTriggersKeepsReplaceTriggeredBy checks that ignoring
// triggers_replace does not clobber a replace_triggered_by on the same
// terraform_data: when the referenced value changes, the resource is still
// replaced and its dependent recreated.
func TestL2TdataIgnoreTriggersKeepsReplaceTriggeredBy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_triggers_keeps_rtb", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}

// TestL2TdataIgnoreTriggersInputUpdate checks that ignoring triggers_replace
// does not suppress an un-ignored input change: the input updates in place (no
// replacement, stable id) while the triggers_replace change is dropped.
func TestL2TdataIgnoreTriggersInputUpdate(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_triggers_input_update", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StageApply}, {Mode: tfcompat.StageApply}},
	})
}
