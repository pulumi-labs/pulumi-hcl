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

// TestL1UUIDV5Namespace pins that `uuidv5` rejects an upper-case namespace
// keyword on both runtimes: OpenTofu only recognizes the lower-case forms
// (`dns`, `url`, `oid`, `x500`) and otherwise tries to parse the argument as a
// literal UUID, so `"DNS"` fails rather than aliasing to the DNS namespace.
func TestL1UUIDV5Namespace(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_uuidv5_namespace", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
output "id" {
  value = uuidv5("DNS", "example.com")
}
`},
			ExpectErr: `uuidv5() doesn't support namespace DNS`,
		}},
	})
}
