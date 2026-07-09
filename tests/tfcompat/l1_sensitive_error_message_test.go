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

// TestL1Validation_SensitiveErrorMessage asserts that a failing variable
// validation whose error_message interpolates a sensitive value is rejected
// the same way on both runtimes. OpenTofu refuses to render the message and
// fails with "Error message refers to sensitive values"; pulumi-hcl must
// produce an equivalent failure rather than leaking the sensitive value.
func TestL1Validation_SensitiveErrorMessage(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_validation_sensitive_error", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			ExpectErr: "Error message refers to sensitive values",
		}},
	})
}
