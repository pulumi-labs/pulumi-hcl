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

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
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
