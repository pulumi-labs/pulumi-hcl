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

// TestL2BlockSingular exercises a TF block whose schema is MaxItems=1 with an
// object element. The bridge flattens this to a single Pulumi object, but TF
// source uses block syntax — the engine must accept the block form and feed
// the bridge a singular object.
func TestL2BlockSingular(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_block_singular", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}

// TestL2BlockMulti exercises a TF block that repeats — the bridge projects it
// as a Pulumi list of objects. The engine collects each `tag {}` instance.
func TestL2BlockMulti(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_block_multi", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}

// TestL2BlockNested exercises a singular block containing a singular block.
// Both flattenings are decided by the inner shim, which the recursive
// BodyMapping must surface.
func TestL2BlockNested(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_block_nested", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
	})
}
