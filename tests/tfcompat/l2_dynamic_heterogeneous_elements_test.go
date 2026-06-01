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

// A `dynamic` block expanding a list whose elements set different subsets of
// the same optional keys. The community-module idiom that triggers this:
//
//	dynamic "ingress" {
//	  for_each = var.rules
//	  content {
//	    cidr_block      = lookup(ingress.value, "cidr_block",      null)
//	    ipv6_cidr_block = lookup(ingress.value, "ipv6_cidr_block", null)
//	  }
//	}
//
// with `var.rules = [{ cidr_block = "..." }, { ipv6_cidr_block = "..." }]`.
// Per-element object types diverge after evaluation; pulumi-hcl must unify
// them before handing to cty.ListVal.
func TestL2DynamicHeterogeneousElements(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_dynamic_heterogeneous_elements", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
