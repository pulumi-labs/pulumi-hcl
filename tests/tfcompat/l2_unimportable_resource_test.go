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

// The resource's read fails on any call, standing in for a resource an import
// cannot read — one whose import string the state's id does not carry, like
// aws_iam_role_policy_attachment's "role/policy". Supplying the instance's
// values from state means the engine never reads it, so the round-trip is
// clean.
func TestL2UnimportableResource(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_unimportable_resource", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "unimportable", Factory: providers.UnimportableProvider},
		},
	})
}
