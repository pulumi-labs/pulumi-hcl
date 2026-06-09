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

// TestL2Moved covers the `moved` block refactoring cases described in the
// OpenTofu docs. A `moved` block records a rename so an existing object is moved
// to the new address rather than replaced; every case below is a state-only
// move that must issue no provider Create/Delete operations on the rename, so
// the two runtimes must agree on the provider operations across both stages.
//
// The subtests tagged "not yet supported" currently FAIL: pulumi-hcl replaces
// the resource because the alias does not match the prior address. They are kept
// as executable documentation of the remaining gaps.
func TestL2Moved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dir  string
	}{
		// Supported.
		{"rename", "l2_moved_rename"},                         // rename a resource
		{"count_for_each_module", "l2_moved_rename_expanded"}, // rename of count/for_each + in-module resources

		// Not yet supported.
		{"enable_count", "l2_moved_enable_count"},             // whole resource -> a[0] (instance key)
		{"rekey_for_each", "l2_moved_rekey"},                  // a["small"] -> a["tiny"] (instance key)
		{"module_call_rename", "l2_moved_module_call_rename"}, // module.a -> module.b
		{"module_call_count", "l2_moved_module_call_count"},   // module.a -> module.a[0]
		{"split_module", "l2_moved_split"},                    // root resource -> module.x (cross-boundary)
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			tfcompat.RunCase(t, c.dir, tfcompat.Case{
				Providers: []tfcompat.Provider{
					{Name: "simple", Factory: providers.SimpleProvider},
				},
			})
		})
	}
}
