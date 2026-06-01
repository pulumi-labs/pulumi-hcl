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

// TestL2TerraformData covers the issue repro: the builtin `terraform` provider
// (terraform_data) has no installable plugin, so both tofu and pulumi-language-hcl
// must handle it internally and produce the same `output`. No providers are
// declared — terraform_data is a builtin on both paths.
func TestL2TerraformData(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_data", tfcompat.Case{})
}
