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

// TestL2ReplaceTriggeredByWholeResource proves that a whole-resource reference
// in `lifecycle.replace_triggered_by` is action-based in OpenTofu but
// value-based in pulumi-hcl.
//
// `dependent` references the entire `middle` resource (not one of its
// attributes). Across the two stages `middle` is replaced -- but its inputs are
// constant, so every attribute of `middle` is identical before and after.
// OpenTofu replaces `dependent` because `middle` is planned for replacement;
// pulumi-hcl does not, because `middle`'s serialized value never changes. The
// provider-op traces therefore differ (tofu records an extra replace of
// `dependent`), and the harness fails.
func TestL2ReplaceTriggeredByWholeResource(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_replace_triggered_by_whole_resource", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "replacer", Factory: providers.ReplacerProvider},
		},
	})
}
