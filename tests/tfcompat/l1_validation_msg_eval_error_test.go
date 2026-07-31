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

// TestL1ValidationMsgEvalError asserts that when a failing variable validation's
// error_message cannot be rendered (here it interpolates a null value, which is
// not allowed in a string template), both runtimes surface that
// template-interpolation failure. OpenTofu reports an "Invalid template
// interpolation value" diagnostic pointing at the error_message; pulumi-hcl
// swallows the evaluation error and falls back to a generic "validation failed"
// message, so the real problem never reaches the user.
func TestL1ValidationMsgEvalError(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_validation_msg_eval_error", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			ExpectErr: "Invalid template interpolation value",
		}},
	})
}
