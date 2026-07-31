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

// OpenTofu reads data sources whose config is fully known during plan, and
// apply starts only after plan completes — so a delayed read of `d` records
// before the create of the completely independent `r`. The recorded op order
// pins that plan-phase reads precede every apply-phase operation, a phase
// barrier rather than a dependency edge.
func TestL2DataPlanReadBarrier(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_plan_read_barrier", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
