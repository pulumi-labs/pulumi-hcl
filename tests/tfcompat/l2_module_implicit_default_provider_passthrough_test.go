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

// TestL2ModuleImplicitDefaultProviderPassthrough pins TF's behaviour for a
// module block whose `providers` argument passes the *implicit* default
// provider — `providers = { simple = simple }` with no `provider "simple"`
// block anywhere in the configuration. TF resolves the reference to the
// implicit (empty) default provider configuration; pulumi-hcl's graph
// builder leaves an unresolved `simple` node behind and fails validation
// with `unknown node "simple"`.
func TestL2ModuleImplicitDefaultProviderPassthrough(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_implicit_default_provider_passthrough", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
