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
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
)

func TestL2PreventDestroy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		AssertState: func(t *testing.T, resources []apitype.ResourceV3) {
			var found *apitype.ResourceV3
			for i, r := range resources {
				if strings.HasSuffix(string(r.URN), "::protected") {
					found = &resources[i]
					break
				}
			}
			if assert.NotNil(t, found, "simple_resource.protected not found in Pulumi state") {
				assert.True(t, found.Protect, "expected Protect=true on simple_resource.protected; got %+v", found)
			}
		},
	})
}
