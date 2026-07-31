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

// TestL2ModuleInheritedProviderRenamedLocalName pins TF's rule that a child
// module inherits its parent's default provider configuration by the
// provider's fully-qualified source address, not by local name. The child
// here calls `hashicorp/simple` by the local name `myp`, so the resource's
// `prefix_result` must still carry the root `provider "simple"` block's
// prefix.
func TestL2ModuleInheritedProviderRenamedLocalName(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_inherited_provider_renamed_local_name", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
