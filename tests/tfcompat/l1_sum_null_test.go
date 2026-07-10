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

// TestL1SumNull asserts that both runtimes reject a null element passed to
// `sum` with the same graceful argument error, rather than panicking inside
// the function implementation as pulumi-hcl previously did.
func TestL1SumNull(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_sum_null", tfcompat.Case{
		Stages: []tfcompat.Stage{
			{ExpectErr: "argument must be list, set, or tuple"},
		},
	})
}
