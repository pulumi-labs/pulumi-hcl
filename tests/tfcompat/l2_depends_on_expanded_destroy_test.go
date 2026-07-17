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

// depends_on naming a whole for_each resource must govern destroy ordering:
// `a["x"]`'s delete waits for `b`'s delayed delete. A runtime that resolves
// depends_on targets by unexpanded address finds no instance state, records
// no dependency, and lets a["x"] delete ahead of b.
func TestL2DependsOnExpandedDestroy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_depends_on_expanded_destroy", tfcompat.Case{
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
