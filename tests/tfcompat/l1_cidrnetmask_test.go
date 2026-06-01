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
)

// TestL1CidrNetmask_IPv6 pins that `cidrnetmask` rejects an IPv6 prefix on both
// runtimes: a netmask is an IPv4-only concept, so OpenTofu errors rather than
// rendering the IPv6 mask.
func TestL1CidrNetmask_IPv6(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_cidrnetmask_ipv6", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
output "mask" {
  value = cidrnetmask("2001:db8::/32")
}
`},
			// OpenTofu line-wraps the diagnostic after "have a", so match only the
		// portion that stays on a single rendered line in both runtimes.
		ExpectErr: "IPv6 addresses cannot have a",
		}},
	})
}
