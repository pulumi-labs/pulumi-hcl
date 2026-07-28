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

// Deleting a resource that carries a `when = destroy` provisioner from the
// configuration must destroy it, without running the provisioner: the block
// went away with the resource block.
func TestL2Provisioner_DestroyOrphaned(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_destroy_orphaned", tfcompat.Case{
		Stages: []tfcompat.Stage{{}, {}},
	})
}
