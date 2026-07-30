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

// preventDestroyErr is the plan-time refusal both runtimes must emit when a
// prevent_destroy resource would be destroyed. Kept short of the full
// sentence because tofu hard-wraps its diagnostics at a position that varies
// with the resource address.
const preventDestroyErr = "has prevent_destroy set"

// TestL2PreventDestroyRemoved removes a guarded resource from configuration.
// The guard is config-gated: removing the block removes the guard with it, so
// a single apply destroys the orphaned instance and succeeds.
func TestL2PreventDestroyRemoved(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_removed", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

// TestL2PreventDestroyReplace forces a replacement of a guarded resource. The
// destroy half of the replacement is refused with the same diagnostic at both
// plan and apply time.
func TestL2PreventDestroyReplace(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_replace", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "replacer", Factory: providers.ReplacerProvider},
		},
		Stages: []tfcompat.Stage{
			{},
			{Mode: tfcompat.StagePreview, ExpectErr: preventDestroyErr},
			{ExpectErr: preventDestroyErr},
		},
	})
}

// TestL2PreventDestroyCount shrinks the count of a guarded resource. The
// orphaned instance's block is still in configuration with the guard set, so
// the apply is refused.
func TestL2PreventDestroyCount(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_count", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{},
			{ExpectErr: preventDestroyErr},
		},
	})
}

// TestL2PreventDestroyCountRef sets prevent_destroy from count.index. The
// guard must be evaluable for instances whose per-instance data is gone, so
// both runtimes reject the reference outright.
func TestL2PreventDestroyCountRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_count_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			// Case-neutral so it matches tofu's capitalized diagnostic and the
			// runtime's lowercase error.
			{ExpectErr: "reference in prevent_destroy"},
		},
	})
}

// TestL2PreventDestroyProvisioner destroys a guarded resource that also
// carries a destroy-time provisioner. The guard blocks the plan before the
// provisioner fires, so the marker file must not exist afterwards.
func TestL2PreventDestroyProvisioner(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_provisioner", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{},
			{Mode: tfcompat.StageDestroy, ExpectErr: preventDestroyErr},
			{},
		},
	})
}

// TestL2PreventDestroy destroys a guarded resource: both runtimes refuse and
// the instance survives.
func TestL2PreventDestroy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{},
			{Mode: tfcompat.StageDestroy, ExpectErr: preventDestroyErr},
		},
	})
}
