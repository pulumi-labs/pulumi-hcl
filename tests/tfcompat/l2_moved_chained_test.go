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

// TestL2MovedChained covers two `moved` blocks that chain in one config: an
// object created as `a` is renamed to `b` and then `b` is renamed to `c`.
// OpenTofu follows the chain (a -> b -> c) and moves the existing object to
// `c` with no create/delete. The resource must not be destroyed and recreated
// across the move, so both runtimes must record only the single initial Create.
func TestL2MovedChained(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_chained", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2MovedModuleChained covers two `moved` blocks that chain on a module
// call: the call created as `module.a` is renamed to `module.b` and then
// `module.b` is renamed to `module.c`. OpenTofu follows the chain and moves
// the existing objects to `module.c` with no create/delete.
func TestL2MovedModuleChained(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_module_chained", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
