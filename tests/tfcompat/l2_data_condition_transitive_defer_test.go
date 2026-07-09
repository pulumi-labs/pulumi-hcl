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

// A data source carrying a pre- or postcondition weighs its whole ancestry,
// not just the blocks its own `depends_on` names, when deciding whether to
// defer its read: a condition can only be satisfied by whatever the pending
// changes upstream produce. The read here reaches a pending resource through
// another data source, so it belongs to apply.
func TestL2DataConditionTransitiveDefer(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_condition_transitive_defer", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pending", Factory: providers.PendingProvider},
		},
		// pending_lookup consults the provider's per-factory-instance record
		// of created things, which the fresh import-check driver starts
		// empty, so its post-import read fails by design.
		SkipImport: "the pending provider's backend is factory-local",

		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{},
		},
	})
}
