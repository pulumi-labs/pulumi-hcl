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

// TestL2DynamicMultiBlockOrder covers two `dynamic` blocks targeting the same
// order-significant (TypeList) block. tofu expands them in source order; pulumi-hcl
// places a later dynamic block's group ahead of an earlier one, reversing the
// groups. The divergence surfaces in the tag list the provider receives.
func TestL2DynamicMultiBlockOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_dynamic_multi_block_order", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
