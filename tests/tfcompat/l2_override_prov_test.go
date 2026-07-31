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

// TestL2OverrideProvForEach covers an override file adding a `for_each` to a
// provider configuration the base declares without one. `for_each` expands
// the declaration rather than configuring it, so an override file cannot set
// it: the provider configuration stays unexpanded and `provider = simple.alt`
// still addresses it.
func TestL2OverrideProvForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_override_prov_for_each_added", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
