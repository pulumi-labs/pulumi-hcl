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
// Renaming a module call still replaces the call's resources, because it
// requires aliasing the module's component resource and re-aliasing every child
// (whose name embeds the module instance name) against a component that no
// longer exists in the run. Those cases are skipped until that is implemented.
func TestL2Moved(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		dir  string
	}{
		// Renaming a resource, including every count/for_each instance, and a
		// resource inside a module.
		{name: "rename", dir: "l2_moved_rename"},
		{name: "count_for_each_module", dir: "l2_moved_rename_expanded"},

		// Changing a resource's instance key (enabling/disabling count/for_each,
		// or rekeying a for_each instance).
		{name: "enable_count", dir: "l2_moved_enable_count"},           // whole resource -> a[0]
		{name: "rekey_for_each", dir: "l2_moved_rekey"},                // a["small"] -> a["tiny"]
		{name: "count_to_for_each", dir: "l2_moved_count_to_for_each"}, // a[0] -> a["x"]

		// Moving a resource across a module boundary.
		{name: "split_module", dir: "l2_moved_split"}, // root resource -> module.x

		// Renaming or re-keying a module call itself.
		{name: "module_call_rename", dir: "l2_moved_module_call_rename"}, // module.a -> module.b
		{name: "module_call_count", dir: "l2_moved_module_call_count"},   // module.a -> module.a[0]

		// Moving a resource from one non-root module to another.
		{name: "module_to_module", dir: "l2_moved_module_to_module"}, // module.a.r -> module.b.r

		// Moving a resource out of a module back to the root.
		{name: "consolidate", dir: "l2_moved_consolidate"}, // module.a.r -> r

		// Renaming a module call composed with a move out of it, in one apply.
		{name: "module_call_rename_out", dir: "l2_moved_module_call_rename_out"}, // module.a.r -> r
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

	// Changing the resource's type. This needs the plugin-framework provider:
	// the target type must implement MoveResourceState, which every
	// terraform-plugin-sdk/v2 provider rejects unconditionally.
	t.Run("change_type", func(t *testing.T) {
		t.Parallel()
		tfcompat.RunCase(t, "l2_moved_change_type", tfcompat.Case{
			Providers: []tfcompat.Provider{
				{Name: "pfx", PFFactory: providers.PFXProvider},
			},
		})
	})
}

