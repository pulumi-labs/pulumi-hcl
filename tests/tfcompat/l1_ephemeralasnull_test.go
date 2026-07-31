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

// ephemeralasnull replaces the ephemeral parts of a value with typed nulls,
// making the result usable in persisted contexts like a non-ephemeral output.
// The deep case (object with one ephemeral attribute), the scalar case, and
// the negative case (values with no ephemeral parts pass through unchanged)
// are all exercised.
func TestL1EphemeralAsNull(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_ephemeralasnull", tfcompat.Case{})
}
