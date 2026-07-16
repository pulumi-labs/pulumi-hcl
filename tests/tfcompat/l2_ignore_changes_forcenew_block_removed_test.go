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

// TestL2IgnoreChangesForceNewBlockRemoved covers ignore_changes on a ForceNew
// attribute inside a MaxItems=1 block when that block is removed. `mode` is
// ForceNew and listed in ignore_changes as settings[0].mode. When stage 1
// removes the whole `settings` block, OpenTofu can no longer reset
// settings[0].mode (the index no longer exists), so it observes the ForceNew
// change and replaces the resource (destroy + create). pulumi-hcl suppresses
// the change and updates the resource in place, so the two runtimes emit a
// different sequence of provider operations.
func TestL2IgnoreChangesForceNewBlockRemoved(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_ignore_changes_forcenew_block_removed", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "fnblock", Factory: providers.FNBlockProvider},
		},
	})
}
