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

// TestL2BlockTypeClash exercises a MaxItems=1 nested block whose bridged
// Pulumi type name is shared with (and overwritten by) an unbounded block of
// another resource, so the schema's property is the plural list while the
// mapping still flattens the singular block.
func TestL2BlockTypeClash(t *testing.T) {
	t.Parallel()
	t.Skip("TODO[https://github.com/pulumi/pulumi-terraform-bridge/issues/3583]: " +
		"tfgen overwrites the shared nested type with the unbounded declaration, " +
		"so the Pulumi schema has no property for the flattened block")
	tfcompat.RunCase(t, "l2_block_type_clash", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "typeclash", Factory: providers.TypeClashProvider},
		},
	})
}
