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

// TestL2ModuleVariableValidation asserts that a `validation` block on a child
// module's input variable runs against the value the parent passes in: an
// invalid value must make both runtimes fail with the configured error message,
// matching OpenTofu.
func TestL2ModuleVariableValidation(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_variable_validation", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			Files: map[string]string{
				"main.tf": `
module "child" {
  source = "./child"
  name   = "x"
}

output "name" {
  value = module.child.name
}
`,
				"child/main.tf": `
variable "name" {
  type = string

  validation {
    condition     = length(var.name) > 3
    error_message = "VALIDATION_FAILED_MODULE_VAR_TOO_SHORT"
  }
}

output "name" {
  value = var.name
}
`,
			},
			ExpectErr: "VALIDATION_FAILED_MODULE_VAR_TOO_SHORT",
		}},
	})
}
