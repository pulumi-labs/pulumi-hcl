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
)

// TestL2TerraformRemoteState reads a local Terraform state file through the
// builtin `terraform` provider's terraform_remote_state data source. OpenTofu
// handles it internally; pulumi-language-hcl lowers the `local` backend onto the
// pulumi-terraform getLocalReference invoke. Both paths read the same
// remote.tfstate fixture and must surface identical `.outputs`. No TF providers
// are declared.
func TestL2TerraformRemoteState(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_remote_state", tfcompat.Case{})
}

// TestL2TerraformRemoteStateDefaults checks that terraform_remote_state's
// `defaults` fills in outputs the referenced state omits while a present output
// keeps its state value. OpenTofu merges defaults internally; pulumi-language-hcl
// overlays them on the getLocalReference result. Both paths must agree.
func TestL2TerraformRemoteStateDefaults(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_remote_state_defaults", tfcompat.Case{})
}
