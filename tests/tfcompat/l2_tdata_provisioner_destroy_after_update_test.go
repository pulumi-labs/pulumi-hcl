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
)

// terraform_data is created with one set, updated in place to another, then
// destroyed. The destroy-time provisioner's self.output must be the
// last-applied input, still typed as a set — not the create-time value frozen
// in the engine's builtin, and not a tuple.
func TestL2TdataProvisionerDestroyAfterUpdate(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_provisioner_destroy_after_update", tfcompat.Case{
		Stages: []tfcompat.Stage{
			{},
			{},
			{Mode: tfcompat.StageDestroy},
		},
	})
}
