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

// TestL2ModuleAliasedProviderPassthrough pins two parser shapes that
// tofu accepts but pulumi-hcl currently rejects:
//
//  1. A dotted key on the LHS of a `providers = { ... }` map entry, e.g.
//     `providers = { simple.foo = simple.from_parent }`.
//
//  2. A traversal inside `configuration_aliases`, e.g.
//     `configuration_aliases = [simple.foo]`.
//
// Both occur together in the canonical pattern for passing an aliased
// provider into a module that declares it explicitly. Either parser
// limitation alone breaks this test.
func TestL2ModuleAliasedProviderPassthrough(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_aliased_provider_passthrough", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
