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

// OpenTofu merges a provisioner's `_` escaping block into its configuration, so
// a `command` written inside `_ { ... }` runs and the apply succeeds.
// pulumi-hcl's provisioner parser only recognizes `connection` sub-blocks and
// leaves the `_` block in the config body, where the local-exec decoder rejects
// it (and finds no command), so the apply errors.
func TestL2ProvisionerEscapeBlock(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_escape_block", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
