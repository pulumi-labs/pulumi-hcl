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

// TestL2Precondition_MultipleFail asserts that when a resource has several
// preconditions and more than one fails, both runtimes report every failing
// rule. OpenTofu evaluates all precondition rules and aggregates their
// diagnostics; pulumi-hcl registers each rule as an independent resource hook
// and aborts on the first failure, so it only surfaces the first message.
func TestL2Precondition_MultipleFail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_precondition_multiple_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: "PRECOND_TWO_FAILS",
		}},
	})
}
