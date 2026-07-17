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

// A count resource indexed by a value computed from a data source
// (`r0[tonumber(data.order_data.idx.result)]`). The dynamic index is a
// whole-resource reference, so `r1` waits for every `r0` instance — including
// the delayed `r0[1]` — even though the resolved index selects `r0[0]`. The
// recorded op order proves the wide dependency, and the stack output proves
// the index still resolves to the right instance's value once every instance
// has registered.
func TestL2RefCountDynamicIndex(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_ref_count_dynamic_index", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
