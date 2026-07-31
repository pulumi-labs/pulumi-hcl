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

// TestL2MaxItemsOneScalarAttr sets a TypeList MaxItems=1 field with a scalar
// Elem — an attribute in TF (`alias = ["x"]`) that the bridge flattens to a
// plain string. The single-element tuple must collapse to the flattened
// scalar on input, and re-expand to a single-element list on output.
func TestL2MaxItemsOneScalarAttr(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_max_items_one_scalar_attr", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
