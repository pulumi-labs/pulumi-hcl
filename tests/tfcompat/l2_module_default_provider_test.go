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

// TestL2ModuleDefaultProvider pins TF's behaviour for an un-aliased
// `provider "simple"` block declared inside a child module: resources in
// that module pick up the in-module block's config (so prefix_result is
// "module-prefix-world"). pulumi-hcl currently ignores in-module default
// provider blocks for resources that don't reference them explicitly,
// falling back to an unconfigured engine default.
func TestL2ModuleDefaultProvider(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_default_provider", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
