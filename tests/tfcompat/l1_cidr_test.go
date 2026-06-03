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

func TestL1Cidr(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_cidr", tfcompat.Case{
		Stages: []tfcompat.Stage{
			{
				Files: map[string]string{"main.tf": `
output "host_pos" {
  value = cidrhost("10.0.0.0/24", 5)
}

output "host_neg_one" {
  value = cidrhost("10.0.0.0/24", -1)
}

output "host_neg_two" {
  value = cidrhost("10.0.0.0/24", -2)
}

output "host_leading_zeros" {
  value = cidrhost("010.001.0.0/24", 5)
}

output "netmask" {
  value = cidrnetmask("10.0.0.0/8")
}

output "netmask_leading_zeros" {
  value = cidrnetmask("010.001.0.0/24")
}

output "subnet_leading_zeros" {
  value = cidrsubnet("010.001.0.0/16", 8, 2)
}

output "subnets_leading_zeros" {
  value = cidrsubnets("010.001.0.0/16", 8, 8)
}
`},
			},
			{
				Files: map[string]string{"main.tf": `
output "mask" {
  value = cidrnetmask("2001:db8::/32")
}
`},
				// OpenTofu line-wraps the diagnostic after "have a", so match
				// only the portion that stays on a single rendered line in both
				// runtimes.
				ExpectErr: "IPv6 addresses cannot have a",
			},
		},
	})
}
