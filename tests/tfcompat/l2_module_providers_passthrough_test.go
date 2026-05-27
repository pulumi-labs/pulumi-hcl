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

// TestL2ModuleProvidersPassthrough pins TF's behaviour for the
// `providers = { simple = simple.alias }` argument on a module block:
// the child's default `simple` provider must resolve to the parent's
// `simple.configured` block, so the resource's `prefix_result` carries
// the parent's `prefix`. pulumi-hcl parses `mod.Providers` but does not
// honour it at runtime — the child ends up with an unconfigured default
// provider and an empty prefix.
func TestL2ModuleProvidersPassthrough(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_providers_passthrough", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
