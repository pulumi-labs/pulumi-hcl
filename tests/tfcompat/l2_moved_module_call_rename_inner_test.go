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
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2MovedModuleCallRenameInner composes two `moved` blocks in one apply: the
// root renames the module call (module.a -> module.b) and the module itself
// renames the resource inside it (simple_resource.old -> simple_resource.new).
// OpenTofu applies both, so module.a.simple_resource.old moves to
// module.b.simple_resource.new with no create or delete.
func TestL2MovedModuleCallRenameInner(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_module_call_rename_inner", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
