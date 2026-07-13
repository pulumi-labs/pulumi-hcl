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

// A partial destroy leaves a resource absent from state while the program still
// declares it and an output indexes it. On the second destroy both runtimes
// re-run the program and fail on the blocker's delete; the bug is that
// `pulumi destroy --run-program` instead fails earlier, unable to evaluate the
// output that now indexes the absent resource, where `tofu destroy` evaluates
// it to null and proceeds. Both destroys are expected to fail with the blocker
// error, so once pulumi tolerates the null index it matches Terraform.
func TestL2RunProgramDestroyPartial(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_run_program_destroy_partial", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "partialdestroy", Factory: providers.PartialDestroyProvider},
		},
		Stages: []tfcompat.Stage{
			{},
			{Mode: tfcompat.StageDestroy, ExpectErr: "intentionally failed"},
			{Mode: tfcompat.StageDestroy, ExpectErr: "intentionally failed"},
		},
	})
}
