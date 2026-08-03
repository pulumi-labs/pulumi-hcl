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

// Two module calls side by side inside one module. Every other multi-call
// case puts the siblings at the root, where each import walks a fresh
// ancestor chain; here the chains share "outer", so the import must still
// emit a component for each sibling below it.
//
// The import shape itself is pinned by TestConvertTFState_SiblingModuleCalls,
// which is what guards that behaviour while the state check is skipped below.
func TestL2ModuleSiblingCalls(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_sibling_calls", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		// TODO[pulumi/pulumi-hcl#167]: two component instances of one type
		// ("outer.a" and "outer.b", both components:index:Leaf) record their
		// inputs non-deterministically, so a preview after an unchanged up
		// intermittently proposes a component update and the convergence
		// assertion flakes. Not an import bug: a fresh up/preview loop on
		// this program, with no import in it at all, oscillates the same way.
		SkipImport: "component inputs do not settle for same-typed sibling modules",
	})
}
