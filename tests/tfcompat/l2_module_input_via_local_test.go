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

// TestL2ModuleInputViaLocal exercises a module call whose input reaches a
// resource only through a root local: the data source inside the module reads
// that input, so its read must defer until the resource is applied rather
// than gate the plan phase.
//
// https://github.com/pulumi/pulumi-hcl/issues/514
func TestL2ModuleInputViaLocal(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_input_via_local", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
