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

// TestL2ReplaceTriggeredBy drives both paths across two stages on the same
// stack. The `dependent` resource references `trigger.result` from a
// `lifecycle.replace_triggered_by` list; stage 1 changes `trigger.input_one`,
// which updates `trigger.result`, which must force `dependent` to be replaced
// even though its own inputs are unchanged. Both tofu and pulumi must produce
// the same provider-op trace across both stages.
func TestL2ReplaceTriggeredBy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_replace_triggered_by", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
