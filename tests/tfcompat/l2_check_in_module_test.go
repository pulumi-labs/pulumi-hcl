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
	"github.com/stretchr/testify/require"
)

// TestL2Check_InModule covers a check block declared inside a child module.
// Terraform evaluates check blocks in every module in the configuration, so a
// failing assertion in a child module surfaces its error_message as a
// non-blocking warning just like a root-module check does.
func TestL2Check_InModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_in_module", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			AssertOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "module check assertion did not hold")
			},
		}},
	})
}
