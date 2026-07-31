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

// TestL2CBDPropagationTransitive checks that create_before_destroy propagates
// across more than one dependency hop. Only the top of a base <- middle <- top
// chain declares it; changing base's ForceNew label replaces all three, and the
// behaviour must reach both middle and base so every create runs before any
// delete. The top records witness = 3 when it does, and a larger number when a
// hop is left at delete-before-replace.
func TestL2CBDPropagationTransitive(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_cbd_propagation_transitive", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "cascade", Factory: providers.CascadeProvider},
		},
	})
}
