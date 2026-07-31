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

// A root variable declared `ephemeral = true` (OpenTofu 1.11+) is accepted by
// `tofu apply`, which proceeds normally. pulumi-hcl's variable schema does not
// list `ephemeral`, so its strict block decode rejects the config with an
// "Unsupported argument" error before anything runs.
func TestL1EphemeralVariable(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_ephemeral_variable", tfcompat.Case{})
}
