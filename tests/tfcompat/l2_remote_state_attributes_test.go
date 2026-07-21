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
)

// TestL2RemoteStateAttributes references the terraform_remote_state attributes
// other than `outputs`. OpenTofu's data source stores `backend`, `config`,
// `workspace` and `defaults` back into its own object, so `.backend` and
// `.config.path` are readable alongside `.outputs`. pulumi-language-hcl lowers
// the data source onto the pulumi-terraform state-reference invoke, whose
// result must expose the same attributes.
func TestL2RemoteStateAttributes(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_remote_state_attributes", tfcompat.Case{})
}
