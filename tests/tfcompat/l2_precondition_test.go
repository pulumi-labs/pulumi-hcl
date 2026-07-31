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

// TestL2Precondition_Pass asserts a passing precondition does not block the
// resource on either runtime.
func TestL2Precondition_Pass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_precondition_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2Precondition_Fail asserts a failing precondition blocks the update on
// both runtimes with the expected error message.
func TestL2Precondition_Fail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_precondition_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: "PRECONDITION_VIOLATED",
		}},
	})
}

// TestL2Precondition_StringBoolCondition asserts that a precondition whose
// condition evaluates to a string convertible to bool ("true"/"false") is
// accepted on both runtimes, matching OpenTofu's bool-conversion of the result.
func TestL2Precondition_StringBoolCondition(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_precondition_string_bool_condition", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2Precondition_UnknownDeferred covers TF's "known after apply" semantics
// end-to-end: the dependent's precondition references the upstream's computed
// output, which is unknown at preview (so both runtimes must defer cleanly)
// and known at apply (so both runtimes must enforce the condition successfully).
func TestL2Precondition_UnknownDeferred(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_precondition_unknown_deferred", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{},
		},
	})
}
