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

// `second` references `first` only through a postcondition condition. Terraform
// derives an implicit dependency from that reference, so on destroy `second`
// is deleted before `first`. destroyorder_resource errors if a second delete
// starts while the first is still in flight, so a missing dependency edge
// surfaces as overlapping deletes. The case applies both resources, then
// destroys them.
func TestL2PostconditionRefDestroyOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_postcondition_ref_destroy_order", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "destroyorder", Factory: providers.DestroyOrderingProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply},
			{Mode: tfcompat.StageDestroy},
		},
	})
}
