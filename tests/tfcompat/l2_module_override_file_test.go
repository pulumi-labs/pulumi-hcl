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

// TestL2ModuleOverrideFile exercises OpenTofu's override-file merge
// semantics: a module directory holding `main.tf` and `override.tf` merges
// the two rather than treating the override as a second declaration of the
// same resource. `input_one` is replaced by the override and `input_two`
// keeps the value from `main.tf`.
func TestL2ModuleOverrideFile(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_override_file", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
