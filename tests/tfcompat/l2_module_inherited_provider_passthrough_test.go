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

// TestL2ModuleInheritedProviderPassthrough asserts parity on a module that
// inherits the root's default provider (no `providers` block on its call) and
// then tries to pass that inherited default down to its own child via a
// `providers` block. OpenTofu rejects this — a module can only pass on a provider
// that was explicitly passed into it, not one it merely inherited — and
// pulumi-hcl must reproduce the same `missing provider` error.
func TestL2ModuleInheritedProviderPassthrough(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_inherited_provider_passthrough", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: `missing provider module.child.provider`,
		}},
	})
}
