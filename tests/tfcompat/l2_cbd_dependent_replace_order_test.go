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

// TestL2CbdDependentReplaceOrder proves that create_before_destroy must not
// propagate to a resource's dependents. `a` declares create_before_destroy and
// is replaced by a ForceNew change; `b` depends on `a` and is replaced too but
// declares no lifecycle. OpenTofu propagates create_before_destroy only to
// dependencies, so `b` keeps delete-before-create ordering and its old instance
// is destroyed before the replacement is created (witness_b = 3). pulumi-hcl
// instead creates every replacement before any delete, so `b` is created before
// its old instance is destroyed (witness_b = 2), a create-before-destroy
// ordering `b` never asked for.
func TestL2CbdDependentReplaceOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_cbd_dependent_replace_order", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "cascade", Factory: providers.CascadeProvider},
		},
	})
}
