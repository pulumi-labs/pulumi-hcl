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

// A body reference to a single instance of a for_each data source whose reads
// are deferred to apply (`data.order_data.d["x"].result`) must create a
// dependency on only that read, not on the delayed `d["y"]` read. The
// recorded op order proves whether apply-phase data reads are instance-narrow
// targets like resources are.
func TestL2RefDeferredDataInstance(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_ref_deferred_data_instance", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
