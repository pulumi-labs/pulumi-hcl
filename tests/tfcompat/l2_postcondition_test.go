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

// TestL2Postcondition_Pass asserts a passing postcondition referencing a
// computed output (self.result) does not block the operation on either runtime.
func TestL2Postcondition_Pass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_postcondition_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "guarded" {
  input_one = "a"
  input_two = false

  lifecycle {
    postcondition {
      condition     = self.result == "a-false"
      error_message = "result must be a-false"
    }
  }
}
`},
		}},
	})
}

// TestL2Postcondition_Fail asserts a failing postcondition surfaces an error
// on both runtimes with the configured message. Unlike preconditions, TF
// still creates the resource — the postcondition fails the deployment after
// the fact, but the resource remains in state.
func TestL2Postcondition_Fail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_postcondition_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "guarded" {
  input_one = "a"
  input_two = false

  lifecycle {
    postcondition {
      condition     = self.result == "expected-different-value"
      error_message = "POSTCONDITION_VIOLATED"
    }
  }
}
`},
			ExpectErr: "POSTCONDITION_VIOLATED",
		}},
	})
}
