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

// The destroy sibling of TestL2DependsOnForEachInstance: destroy dependencies
// are resource-wide in tofu, so an instance-addressed depends_on (`a["x"]`)
// makes BOTH instances' deletes wait for `b`. The recorded op order proves
// the persisted dependency covers every registered instance of the target,
// including expanded targets whose instance outputs are not addressable by
// the block key.
func TestL2DependsOnForEachInstanceDestroy(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_depends_on_for_each_instance_destroy", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply},
			{Mode: tfcompat.StageDestroy},
		},
		OrderDeterministic: true,
	})
}
