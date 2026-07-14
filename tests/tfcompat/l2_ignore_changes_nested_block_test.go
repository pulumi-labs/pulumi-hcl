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

// TestL2IgnoreChangesNestedBlock exercises `ignore_changes` on an attribute
// inside a MaxItems=1 nested block. In TF the block is list-shaped, so the
// path is written `settings[0].mode`; the bridge flattens the block to a
// single Pulumi object (`settings.mode`), so the list index must be dropped
// when translating the ignore_changes path. Stage 1 changes only the ignored
// attribute, so OpenTofu plans no update and the stored value stays "a".
func TestL2IgnoreChangesNestedBlock(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_ignore_changes_nested_block", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
