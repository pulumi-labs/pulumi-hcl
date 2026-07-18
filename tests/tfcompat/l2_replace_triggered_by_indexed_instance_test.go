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

// TestL2ReplaceTriggeredByIndexedInstance proves that an indexed instance
// reference in `lifecycle.replace_triggered_by` is not scoped to that instance
// in pulumi-hcl.
//
// `dependent` references a single indexed instance -- `pool[1]` -- of the
// counted `pool` resource. Between the two stages only pool[0]'s ForceNew
// `force` input changes, so pool[0] is replaced while pool[1] is untouched.
//
// OpenTofu scopes the indexed reference to that one instance: pool[1] never
// changes, so `dependent` is left alone (only pool[0] is replaced). pulumi-hcl
// treats the reference as covering the whole counted resource, so pool[0]'s
// replacement spuriously replaces `dependent`. The provider-op traces differ
// (pulumi records an extra create+delete of `dependent`), and the harness
// fails.
func TestL2ReplaceTriggeredByIndexedInstance(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_replace_triggered_by_indexed_instance", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "replacer", Factory: providers.ReplacerProvider},
		},
	})
}
