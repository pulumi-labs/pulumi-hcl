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

// Like TestL2ModuleForEachInstanceFlow but with the module-internal `a` → `b`
// reference routed through a local. Verified against real tofu: the local
// WIDENS the edge across module instances — both `b`s wait for the delayed
// m["y"].a (unlike a direct reference, which stays per-instance). Guards
// against over-narrowing module locals.
func TestL2ModuleLocalInstanceFlow(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_local_instance_flow", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
