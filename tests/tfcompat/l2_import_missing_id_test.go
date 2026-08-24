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
	"github.com/stretchr/testify/assert"
)

// pfx_anon declares no `id` attribute, so its Terraform state carries nothing
// for an import to key on and the bridge falls back to a sentinel ID. The
// import must invent the same one: AssertState runs over both the created and
// the imported state, so the two IDs are pinned against each other rather than
// against the literal alone.
func TestL2ImportMissingID(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_import_missing_id", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pfx", PFFactory: providers.PFXProvider},
		},
		AssertState: func(t *testing.T, resources []apitype.ResourceV3) {
			for _, r := range resources {
				if r.Type == "pfx:index/anon:Anon" {
					assert.Equal(t, "missing ID", string(r.ID))
					return
				}
			}
			t.Error("no pfx:index/anon:Anon resource in state")
		},
	})
}
