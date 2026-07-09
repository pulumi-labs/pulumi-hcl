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

// TestL1Validation_StringBoolCondition asserts that a variable validation whose
// condition evaluates to a string convertible to bool ("true"/"false") is
// accepted on both runtimes, matching OpenTofu's bool-conversion of the result.
func TestL1Validation_StringBoolCondition(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_validation_string_bool", tfcompat.Case{})
}

// TestL1Validation_Fail asserts that a variable whose validation condition is
// false makes both runtimes fail with the configured error message, matching
// OpenTofu's behaviour.
func TestL1Validation_Fail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_validation_fail", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			ExpectErr: "VALIDATION_FAILED_NAME_TOO_SHORT",
		}},
	})
}
