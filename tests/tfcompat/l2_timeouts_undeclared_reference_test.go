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

// TestL2TimeoutsUndeclaredReference: Reading a timeout for an operation the resource does not declare is an unsupported attribute.
func TestL2TimeoutsUndeclaredReference(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_timeouts_undeclared_reference", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "timeoutable", Factory: providers.TimeoutableProvider},
		},
		Stages: []tfcompat.Stage{{ExpectErr: "Unsupported attribute"}},
	})
}
