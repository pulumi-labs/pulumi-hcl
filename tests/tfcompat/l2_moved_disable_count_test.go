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
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2MovedDisableCount moves a counted instance back to a bare resource
// (`simple_resource.a[0]` -> `simple_resource.a`, disabling count). OpenTofu
// treats this as a state-only move: the object keeps its identity and the
// rename issues no provider Create/Delete. pulumi-hcl instead creates a fresh
// `simple_resource.a` and deletes `simple_resource.a[0]`, so the two runtimes
// disagree on the provider operations. The inverse direction (enabling count,
// `a` -> `a[0]`) is already covered and works.
func TestL2MovedDisableCount(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_disable_count", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
