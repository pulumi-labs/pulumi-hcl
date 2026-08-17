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
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestL2ProviderConfigRef pins the dependency metadata of a provider whose
// configuration consumes a resource output. The metadata preserves the
// reverse ordering needed by a later state-driven destroy.
func TestL2ProviderConfigRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provider_config_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		AssertState: func(t *testing.T, resources []apitype.ResourceV3) {
			var config, configuredProvider *apitype.ResourceV3
			for i := range resources {
				r := &resources[i]
				switch {
				case r.Type == "pulumi:providers:order":
					configuredProvider = r
				case r.URN.Name() == "config":
					config = r
				}
			}
			require.NotNil(t, config, "configuration resource should be in state")
			require.NotNil(t, configuredProvider, "configured provider should be in state")
			assert.Equal(t, []resource.URN{config.URN}, configuredProvider.Dependencies)
			assert.Equal(t,
				[]resource.URN{config.URN},
				configuredProvider.PropertyDependencies[resource.PropertyKey("token")])
		},
	})
}
