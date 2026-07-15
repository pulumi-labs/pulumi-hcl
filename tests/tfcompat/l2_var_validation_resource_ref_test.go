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

// TestL2VarValidation_ResourceRef covers a variable validation whose condition
// references the variable itself and a managed resource's computed output
// (allowed by OpenTofu as long as the condition also refers to the variable).
// OpenTofu orders the rule after the resource and applies cleanly; pulumi-hcl
// fails the whole operation with "Cycle found".
func TestL2VarValidation_ResourceRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_var_validation_resource_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
