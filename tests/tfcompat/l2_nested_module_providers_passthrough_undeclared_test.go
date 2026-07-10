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

// TestL2NestedModuleProvidersPassthroughUndeclared pins TF's behaviour for a
// mid-level module passing `providers = { simple = simple }` while having no
// provider configuration of its own: provider configurations are never
// inherited into a `providers` argument, so both runtimes must fail with the
// same module-qualified missing-provider error even though the root has a
// `provider "simple"` block.
func TestL2NestedModuleProvidersPassthroughUndeclared(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_nested_module_providers_passthrough_undeclared", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: `missing provider module.mid.provider["registry.opentofu.org/hashicorp/simple"]`,
		}},
	})
}
