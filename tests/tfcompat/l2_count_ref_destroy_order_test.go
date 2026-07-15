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

// TestL2CountRefDestroyOrder proves that a reference expressed only through a
// resource's `count` meta-argument establishes a dependency that governs
// destroy ordering. `b`'s `count` references `a`; nothing in `b`'s body
// references `a`. Terraform derives the dependency, so the recorded sequence
// is [create a, create b, delete b, delete a]; OrderDeterministic asserts it,
// with the op that must complete first in each phase delayed so a missing
// edge flips the recorded order deterministically.
func TestL2CountRefDestroyOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_count_ref_destroy_order", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply},
			{Mode: tfcompat.StageDestroy},
		},
		OrderDeterministic: true,
	})
}
