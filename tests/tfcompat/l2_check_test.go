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
	"github.com/stretchr/testify/require"
)

// TestL2Check_Pass asserts a top-level check block whose assertion holds runs
// cleanly on both runtimes, does not affect the resource or its output, and
// surfaces no assertion warning.
func TestL2Check_Pass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			AssertOutput: func(t *testing.T, output string) {
				require.NotContains(t, output, "result did not match expected value")
			},
		}},
	})
}

// TestL2Check_FailIsNonBlocking is the key behavior for #234: a failed check
// assertion reports a warning but does not block the operation. Both runtimes
// must still apply successfully, produce identical outputs, and surface the
// assertion's error_message — unlike a precondition, which fails the update.
func TestL2Check_FailIsNonBlocking(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_fail_non_blocking", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			AssertOutput: func(t *testing.T, output string) {
				require.Contains(t, output, "result did not match expected value")
			},
		}},
	})
}

// TestL2Check_UnknownDeferred covers TF's "known after apply" handling: at
// preview the assertion references the resource's computed result, which is
// unknown, so both runtimes must defer the check without error; at apply the
// value is known and the assertion holds.
func TestL2Check_UnknownDeferred(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_unknown_deferred", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{},
		},
	})
}

// TestL2Check_ScopedDataSource proves a check's nested data source is read, is
// visible to the check's assertion, and — crucially — runs after resources are
// constructed. Creating mark_resource flips an in-memory mark that the scoped
// mark_probe data source reports; the assertion requires it to be set, so it
// holds only if the check ran after the resource was created. No warning is
// surfaced on either runtime.
func TestL2Check_ScopedDataSource(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_scoped_data_source", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "mark", Factory: providers.MarkProvider},
		},
		Stages: []tfcompat.Stage{{
			AssertOutput: func(t *testing.T, output string) {
				require.NotContains(t, output, "scoped data source ran before")
				require.NotContains(t, output, "could not evaluate condition")
			},
		}},
	})
}

// TestL2Check_MultipleDataSourcesRejected locks in that a check block may
// declare at most one scoped data source: both runtimes reject a second one.
func TestL2Check_MultipleDataSourcesRejected(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_check_multiple_data_sources", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			ExpectErr: "Multiple data resource blocks",
		}},
	})
}
