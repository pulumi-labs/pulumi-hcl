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

// A `data` block that depends on a managed resource with changes pending must
// not be read during the plan/preview phase, even when every input value is
// already known: `tofu plan` defers the read to apply both for a direct
// reference to the pending resource and for an explicit `depends_on`. Before
// the fix, `pulumi preview` invoked both reads eagerly (the inputs are known
// strings), recording extra data-source operations — and failing outright for
// providers whose read requires the dependency to exist (issue #266).
//
// Only *direct* references defer: `via_local` reaches the pending resource
// through a `locals` indirection, so both runtimes read it during the
// plan/preview phase (two recorded reads: one per stage) — the recorded op
// counts pin that boundary on both sides.
func TestL2DataSourcePendingDependencyPreview(t *testing.T) {
	t.Parallel()
	files := map[string]string{"main.tf": `
resource "simple_resource" "upstream" {
  input_one = "a"
  input_two = true
}

data "simple_lookup" "by_ref" {
  query = simple_resource.upstream.input_one
}

data "simple_lookup" "by_depends_on" {
  query      = "static"
  depends_on = [simple_resource.upstream]
}

locals {
  indirect = "${simple_resource.upstream.input_one}-local"
}

data "simple_lookup" "via_local" {
  query = local.indirect
}

output "by_ref" {
  value = data.simple_lookup.by_ref.prefix_result
}

output "by_depends_on" {
  value = data.simple_lookup.by_depends_on.prefix_result
}

output "via_local" {
  value = data.simple_lookup.via_local.prefix_result
}
`}
	tfcompat.RunCase(t, "l2_data_source_pending_dependency_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview, Files: files},
			{Mode: tfcompat.StageApply, Files: files},
		},
	})
}
