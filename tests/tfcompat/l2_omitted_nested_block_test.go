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

// TestL2OmittedNestedBlock exercises an optional nested block (`rule`,
// MaxItems=1) left out of an enclosing `policy` block. OpenTofu materializes the
// absent nested block as an empty list of blocks (`[]`, length 0). pulumi-hcl
// materializes it as a single-element list containing null (`[null]`, length 1),
// so reading `policy[0].rule` — or `length(policy[0].rule)` — diverges.
func TestL2OmittedNestedBlock(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_omitted_nested_block", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
