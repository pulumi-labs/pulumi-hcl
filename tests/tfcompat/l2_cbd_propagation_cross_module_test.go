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

// TestL2CBDPropagationCrossModule checks that create_before_destroy propagates
// across a module boundary. The declaring resource is inside a child module and
// depends on a root resource through a module input; changing the root
// resource's ForceNew label replaces both, so the behaviour must reach the root
// resource. The in-module child records witness = 2 when every create runs
// before any delete, and a larger number otherwise.
func TestL2CBDPropagationCrossModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_cbd_propagation_cross_module", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "cascade", Factory: providers.CascadeProvider},
		},
	})
}
