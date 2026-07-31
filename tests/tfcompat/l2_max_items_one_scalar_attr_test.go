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
// scalar on input, and re-expand to a single-element list on output. An
// explicitly-empty assignment stays an empty list, distinct from an unset
// field, which is null.
func TestL2MaxItemsOneScalarAttr(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_max_items_one_scalar_attr", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}

// TestL2MaxItemsOneListElemAttr flattens a MaxItems=1 field whose element is
// itself a list, so the flattened Pulumi value is collection-shaped. OpenTofu
// handles the schema natively, but the bridge's flat encoding is ambiguous
// for it: makeTerraformInput passes arrays through as already-TF-shaped while
// Check re-flattens, mis-nesting the value and panicking on two inner items.
func TestL2MaxItemsOneListElemAttr(t *testing.T) {
	t.Parallel()
	t.Skip("TODO[github.com/pulumi/pulumi-terraform-bridge#3557]: " +
		"the bridge cannot round-trip a MaxItemsOne field with a list-typed Elem; " +
		"Check panics with \"Unexpected multiple elements in array with MaxItems=1\"")
	tfcompat.RunCase(t, "l2_max_items_one_list_elem_attr", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}

// TestL2MaxItemsOneAttrIgnoreChanges indexes into a flattened attribute from
// `ignore_changes`: the TF path `alias[0]` must drop its index like a
// singular block's would, so the change to the flattened value is ignored.
func TestL2MaxItemsOneAttrIgnoreChanges(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_max_items_one_attr_ignore_changes", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}

// TestL2MaxItemsOneAttrTooMany assigns two elements to a MaxItems=1
// attribute; both runtimes must reject it with the one-item limit.
func TestL2MaxItemsOneAttrTooMany(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_max_items_one_attr_too_many", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
		Stages: []tfcompat.Stage{{ExpectErr: "1 item maximum"}},
	})
}

// TestL2ScalarAttrListValue assigns a list to a plain (non-MaxItemsOne)
// string attribute; both runtimes must reject the value rather than coerce
// it.
func TestL2ScalarAttrListValue(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_scalar_attr_list_value", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
		Stages: []tfcompat.Stage{{ExpectErr: "value type"}},
	})
}
