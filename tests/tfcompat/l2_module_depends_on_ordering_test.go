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

// A module referenced only through depends_on (no data flow) must be created
// before the module that depends on it. Terraform serializes on module
// depends_on, so the recorded op sequence is [create producer, create
// consumer]; OrderDeterministic asserts it. The producer's create is delayed,
// so a missing edge flips the recorded order deterministically.
func TestL2Module_DependsOnOrdering(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_depends_on_ordering", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
