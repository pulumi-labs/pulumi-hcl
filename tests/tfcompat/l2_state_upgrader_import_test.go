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

// The resource is at schema version 2, its read cannot recover `note`, and its
// state upgraders rewrite `note` whenever they run. Adopting its state is
// therefore only a no-op if the import carries both the instance's values and
// the version those values are at: an id-only import reads the placeholder
// back, and an import that drops the version runs the upgrade chain from 0.
func TestL2StateUpgraderImport(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_state_upgrader_import", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "upgrader", Factory: providers.UpgraderProvider},
		},
	})
}
