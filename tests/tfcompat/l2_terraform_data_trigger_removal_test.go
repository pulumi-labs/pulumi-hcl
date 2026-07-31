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

// TestL2TerraformDataTriggerRemoval removes terraform_data's triggers_replace
// between stages 0 and 1 ("x" -> absent), then re-adds it in stage 2
// (absent -> "y"). OpenTofu treats clearing or setting this ForceNew attribute
// as a change and replaces the resource, rolling its id and replacing the
// dependent that watches it via replace_triggered_by. Both runtimes must
// produce the same simple provider op trace.
func TestL2TerraformDataTriggerRemoval(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_data_trigger_removal", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
