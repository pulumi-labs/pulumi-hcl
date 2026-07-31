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

// TestL1OutputPrecondition asserts that a failing `precondition` on a top-level
// output makes both runtimes fail with the configured error message, matching
// OpenTofu.
func TestL1OutputPrecondition(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_output_precondition", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			ExpectErr: "OUTPUT_PRECONDITION_FAILED",
		}},
	})
}

// TestL2ModuleOutputPrecondition asserts that a failing `precondition` on a
// child module's output makes both runtimes fail with the configured error
// message, matching OpenTofu.
func TestL2ModuleOutputPrecondition(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_output_precondition", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			ExpectErr: "MODULE_OUTPUT_PRECONDITION_FAILED",
		}},
	})
}
