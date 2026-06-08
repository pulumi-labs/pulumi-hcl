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

// TestL2ProviderInheritance pins TF's default-provider inheritance for child
// modules. The root configures `simple` with prefix = "root-prefix"; each child
// exercises one inheritance property:
//
//   - p2_plain: a child with no provider block inherits the root config.
//   - p3_required_providers: a `required_providers` block (no config block)
//     does not stop inheritance.
//   - p4_grandchild: inheritance is recursive through a nested module.
//   - p6_data_source: a data source inherits the root config.
//
// All four reproduce https://github.com/pulumi-labs/pulumi-hcl/issues/236:
// pulumi-hcl synthesizes a fresh, unconfigured default provider per module
// instead of inheriting the root's, dropping the prefix.
func TestL2ProviderInheritance(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provider_inheritance", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
