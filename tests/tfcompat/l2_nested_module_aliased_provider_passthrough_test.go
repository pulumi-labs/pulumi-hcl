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

// Root passes an aliased provider (simple.special, prefix "special") into
// module "outer" via a providers map. "outer" re-passes its inherited simple
// provider down to nested module "inner", whose resource reads the provider's
// prefix. OpenTofu carries the aliased configuration through both hops, so the
// resource sees prefix "special"; pulumi drops the aliased config on the second
// hop and configures the nested resource with an empty provider.
func TestL2NestedModuleAliasedProviderPassthrough(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_nested_module_aliased_provider_passthrough", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
