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

// TestL2MovedRenameExpanded renames a `count` resource, a `for_each` resource,
// and a resource inside a local module via `moved` blocks (the module's move is
// declared in the module's own configuration). Every instance is a state-only
// move, so neither runtime should issue Create/Delete operations on the rename;
// the alias must match the old name in each context (old-0, old-"x", m-old)
// rather than replace.
func TestL2MovedRenameExpanded(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_rename_expanded", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
