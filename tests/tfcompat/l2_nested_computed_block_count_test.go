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

// TestL2NestedComputedBlockCount traverses a counted resource's computed
// nested blocks (`r.this[0].identity[0].oidc[0].issuer`) while the resource
// is unknown during preview.
func TestL2NestedComputedBlockCount(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_nested_computed_block_count", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "nested", Factory: providers.NestedProvider},
		},
		Stages: []tfcompat.Stage{{
			Mode: tfcompat.StagePreview,
		}},
	})
}
