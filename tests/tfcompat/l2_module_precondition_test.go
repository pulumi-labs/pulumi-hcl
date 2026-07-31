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

// TestL2ModulePrecondition_Pass asserts a passing resource `precondition`
// inside a child module does not block the resource on either runtime.
func TestL2ModulePrecondition_Pass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_precondition_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2ModulePrecondition_Fail asserts a failing resource `precondition`
// inside a child module blocks the update on both runtimes with the configured
// error message.
func TestL2ModulePrecondition_Fail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_precondition_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: "MODULE_RESOURCE_PRECONDITION_FAILED",
		}},
	})
}
