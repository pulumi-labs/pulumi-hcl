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

// TestL1PlanTimestamp exercises the `plantimestamp` built-in. OpenTofu evaluates
// it to the plan-time RFC3339 timestamp; the outputs derive deterministic facts
// from that value so both runtimes can be compared.
func TestL1PlanTimestamp(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_plantimestamp", tfcompat.Case{})
}
