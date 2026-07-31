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

// TestL2DecodeTfvars covers the builtin `terraform` provider's
// provider::terraform::decode_tfvars function
// (https://github.com/pulumi/pulumi-hcl/issues/190): both runtimes must
// parse .tfvars text — literal and resource-produced alike — into identical
// objects, with no plugin to install.
func TestL2DecodeTfvars(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_decode_tfvars", tfcompat.Case{})
}
