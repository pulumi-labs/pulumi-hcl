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

// TestL2TerraformRemoteStateLocalWorkspace reads a non-default workspace's
// state through the builtin terraform_remote_state data source on the local
// backend. OpenTofu's local backend supports named workspaces: with
// `workspace = "staging"` it reads `<workspace_dir>/staging/terraform.tfstate`.
// pulumi-language-hcl rejects the `workspace` attribute on the local backend
// outright, so both paths should agree but do not.
func TestL2TerraformRemoteStateLocalWorkspace(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_remote_state_local_workspace", tfcompat.Case{})
}
