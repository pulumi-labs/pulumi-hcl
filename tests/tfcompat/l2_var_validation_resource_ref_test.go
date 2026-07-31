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

	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2VarValidation_ResourceRef covers a variable validation whose condition
// references the variable itself and a managed resource's computed output.
// The rule is ordered after the resource (deferred at preview, checked at
// apply) and the program applies cleanly, matching OpenTofu.
func TestL2VarValidation_ResourceRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_var_validation_resource_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2ModuleVarValidation_ResourceRef asserts the same for a variable
// declared in a child module whose validation references a resource in that
// module.
func TestL2ModuleVarValidation_ResourceRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_var_validation_resource_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
