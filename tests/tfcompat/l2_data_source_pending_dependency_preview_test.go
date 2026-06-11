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
			// A clean preview after the apply: nothing is pending anymore, so
			// neither runtime defers — all three data sources are read during
			// the plan/preview phase.
			{Mode: tfcompat.StagePreview, Files: files},
		},
	})
}

// A pending *update* must defer dependent data source reads the same way a
// pending create does: after the first apply, changing `input_one` gives the
// resource a pending change, and `tofu plan` defers the read of a data source
// that references it even though the new value is already known.
//
// Skipped: an update that changes only inputs is invisible to the language
// host — the RegisterResourceResponse for an update preview carries a known
// id and known outputs, indistinguishable from a no-op registration — so
// pulumi reads the data source during the preview (one extra recorded read
// with query "b"). Detecting this case needs the engine to report whether a
// registration has a change planned.
func TestL2DataSourcePendingUpdatePreview(t *testing.T) {
	t.Skip("https://github.com/pulumi-labs/pulumi-hcl/issues/269: an input-only pending update is indistinguishable from a no-op, so the read is not deferred")
	t.Parallel()
	program := func(inputOne string) map[string]string {
		return map[string]string{"main.tf": `
resource "simple_resource" "upstream" {
  input_one = "` + inputOne + `"
  input_two = true
}

data "simple_lookup" "by_ref" {
  query = simple_resource.upstream.input_one
}

output "by_ref" {
  value = data.simple_lookup.by_ref.prefix_result
}
`}
	}
	tfcompat.RunCase(t, "l2_data_source_pending_update_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply, Files: program("a")},
			{Mode: tfcompat.StagePreview, Files: program("b")},
			{Mode: tfcompat.StageApply, Files: program("b")},
		},
	})
}

// A pending *replacement* (ForceNew change) must defer dependent data source
// reads: changing `input_replace` plans a replace of the resource, so the
// read of a data source referencing it waits for the apply phase even though
// the referenced value (`input_one`) is unchanged and known.
func TestL2DataSourcePendingReplacePreview(t *testing.T) {
	t.Parallel()
	program := func(replace string) map[string]string {
		return map[string]string{"main.tf": `
resource "simple_resource" "upstream" {
  input_one     = "a"
  input_two     = true
  input_replace = "` + replace + `"
}

data "simple_lookup" "by_ref" {
  query = simple_resource.upstream.input_one
}

output "by_ref" {
  value = data.simple_lookup.by_ref.prefix_result
}
`}
	}
	tfcompat.RunCase(t, "l2_data_source_pending_replace_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply, Files: program("x")},
			{Mode: tfcompat.StagePreview, Files: program("y")},
			{Mode: tfcompat.StageApply, Files: program("y")},
		},
	})
}

// `depends_on = [module.maker]` on a data source must defer the read while
// any managed resource inside that module has a change pending, mirroring
// the transitive walk OpenTofu performs for module references in depends_on.
func TestL2DataSourceModuleDependsOnPendingPreview(t *testing.T) {
	t.Parallel()
	files := map[string]string{
		"main.tf": `
module "maker" {
  source = "./mod"
}

data "simple_lookup" "after_module" {
  query      = "static"
  depends_on = [module.maker]
}

output "after_module" {
  value = data.simple_lookup.after_module.prefix_result
}
`,
		"mod/main.tf": `
resource "simple_resource" "inner" {
  input_one = "m"
  input_two = false
}
`,
	}
	tfcompat.RunCase(t, "l2_data_source_module_depends_on_pending_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview, Files: files},
			{Mode: tfcompat.StageApply, Files: files},
		},
	})
}

// Pending changes are tracked per module *instance*: growing a module call
// from count = 1 to count = 2 leaves instance [0] unchanged, so the data
// source inside instance [0] is still read during the plan/preview phase,
// while the one inside the new instance [1] defers until apply.
func TestL2DataSourceModuleInstancePendingPreview(t *testing.T) {
	t.Parallel()
	program := func(count string) map[string]string {
		return map[string]string{
			"main.tf": `
module "m" {
  source = "./mod"
  count  = ` + count + `
}

output "looked" {
  value = module.m[0].looked
}
`,
			"mod/main.tf": `
resource "simple_resource" "inner" {
  input_one = "i"
  input_two = true
}

data "simple_lookup" "sibling" {
  query = simple_resource.inner.input_one
}

output "looked" {
  value = data.simple_lookup.sibling.prefix_result
}
`,
		}
	}
	tfcompat.RunCase(t, "l2_data_source_module_instance_pending_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply, Files: program("1")},
			{Mode: tfcompat.StagePreview, Files: program("2")},
			{Mode: tfcompat.StageApply, Files: program("2")},
		},
	})
}
