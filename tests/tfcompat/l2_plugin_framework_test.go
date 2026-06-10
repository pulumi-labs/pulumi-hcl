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

// TestL2PluginFrameworkProvider exercises a provider built on
// terraform-plugin-framework (instead of terraform-plugin-sdk/v2) end-to-end:
// provider config, resource create and update, and a data-source read. The
// two stages drive a create then an update so the protocol-level recorder
// compares both operation kinds across the runtimes.
func TestL2PluginFrameworkProvider(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_plugin_framework", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
	})
}
