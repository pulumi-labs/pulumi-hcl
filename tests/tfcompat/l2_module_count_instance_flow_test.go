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

// The count sibling of TestL2ModuleForEachInstanceFlow: a reference inside a
// count module resolves within the same module instance, so m[0].b waits only
// for m[0].a, never for the delayed m[1].a. The recorded op order proves
// whether the runtime keeps module-internal edges per module instance for
// count-keyed instances too.
func TestL2ModuleCountInstanceFlow(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_count_instance_flow", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
