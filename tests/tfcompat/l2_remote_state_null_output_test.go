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
)

// TestL2RemoteStateNullOutput reads a terraform_remote_state whose state stores
// a null-valued output (typed string). OpenTofu preserves the null output, so
// the outputs object keeps the `nullstr` key with a null value. Both paths must
// agree on the rendered outputs object.
func TestL2RemoteStateNullOutput(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_remote_state_null_output", tfcompat.Case{})
}
