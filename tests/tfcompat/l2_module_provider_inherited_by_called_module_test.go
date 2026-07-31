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

// TestL2ModuleProviderInheritedByCalledModule pins TF's behaviour for a default
// `provider` block declared in a non-root module: the `mid` module configures
// `simple` with prefix = "mid-prefix" and calls a grandchild module (with no
// provider block of its own), which inherits that config, so the grandchild's
// resource produces "mid-prefix-p5".
//
// pulumi-hcl synthesizes a fresh, unconfigured default provider for the called
// module instead of inheriting `mid`'s, dropping the prefix.
func TestL2ModuleProviderInheritedByCalledModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_provider_inherited_by_called_module", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
