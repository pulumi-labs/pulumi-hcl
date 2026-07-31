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
)

// TestL2RemoteStateSetOutput reads a terraform_remote_state output typed
// `set(string)` whose stored array order is not sorted. OpenTofu reconstructs
// it as a cty set, so referencing the output renders in canonical sorted order
// regardless of storage order. Both paths must agree.
func TestL2RemoteStateSetOutput(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_remote_state_set_output", tfcompat.Case{})
}
