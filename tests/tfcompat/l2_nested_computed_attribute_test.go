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

// TestL2NestedComputedAttribute traverses a computed nested list attribute
// (`attr[0].nested_attr[0].value`) while the resource is unknown during
// preview. Unlike l2_nested_computed_block, the nesting is an object-typed
// attribute, not an Elem-Resource block.
func TestL2NestedComputedAttribute(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_nested_computed_attribute", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{},
		},
	})
}
