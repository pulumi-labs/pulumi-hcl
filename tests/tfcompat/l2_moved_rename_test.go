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

// A `moved` block renames a resource in place. OpenTofu performs a state-only
// move: the second apply issues NO provider Create/Delete, just records the new
// address. pulumi-hcl maps the move to a Pulumi alias; if the alias does not
// match the renamed resource, pulumi tears down the old resource and creates a
// new one, producing extra Create+Delete provider operations that OpenTofu
// never issues.
//
// Stage 0 declares simple_resource.old; stage 1 renames it to
// simple_resource.new with a `moved` block. The two runtimes must issue the
// same provider operations across both stages.
func TestL2MovedRename(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_moved_rename", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
