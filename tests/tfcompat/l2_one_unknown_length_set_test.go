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

// `one` applied to a set whose length is not yet known must defer to an
// unknown value under `pulumi preview`, just as it does under `tofu plan`.
// Here the set is built from a not-yet-created resource's computed `result`
// plus a literal, so its length could be one (if they collapse) or two.
// OpenTofu's `one` checks whether the length is known and returns an unknown
// value when it isn't, so the plan succeeds. Before the fix, pulumi-hcl's
// `one` took the element count at face value, decided the set had "more than
// one element", and failed the preview.
func TestL2OneUnknownLengthSet(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_one_unknown_length_set", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Mode: tfcompat.StagePreview,
			Files: map[string]string{"main.tf": `
resource "simple_resource" "upstream" {
  input_one = "a"
  input_two = true
}

output "the_one" {
  value = one(toset([simple_resource.upstream.result, "fixed"]))
}
`},
		}},
	})
}
