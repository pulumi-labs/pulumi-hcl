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

// Removed blocks for resources inside modules, in every declaration shape:
// the child module declares the removal of its own resource, the root
// declares a module-prefixed removal into a still-live module, and the root
// declares a removal under a module call that is itself gone. Stage 2's
// outputs read the marker each removed block's destroy-time provisioner
// touched in stage 1.
func TestL2RemovedModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_removed_module", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{}, {}, {}},
	})
}
