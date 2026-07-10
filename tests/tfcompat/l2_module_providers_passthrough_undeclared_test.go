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

// TestL2ModuleProvidersPassthroughUndeclared pins TF's behaviour for a module
// call passing `providers = { simple = simple }` when the root neither
// configures the provider nor declares it in `required_providers`: both
// runtimes must fail with the same missing-provider error.
func TestL2ModuleProvidersPassthroughUndeclared(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_providers_passthrough_undeclared", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: `missing provider provider["registry.opentofu.org/hashicorp/simple"]`,
		}},
	})
}
