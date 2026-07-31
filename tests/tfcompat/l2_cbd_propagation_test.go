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

// TestL2CBDPropagation covers the rule that create_before_destroy propagates to
// a resource's dependencies. The child sets create_before_destroy and references
// the parent's ForceNew output, so changing the parent's label replaces both.
//
// When the propagation is applied, all creates run before any deletes, so the
// replacement child records witness = 2; without it the chain resolves to
// delete-before-replace and the child records witness = 4. The child's witness
// stack output surfaces the divergence (the operation recorder alone cannot, as
// it compares the create/delete multiset order-independently).
func TestL2CBDPropagation(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_cbd_propagation", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "cascade", Factory: providers.CascadeProvider},
		},
	})
}
