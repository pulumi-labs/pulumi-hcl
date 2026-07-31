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
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// A `count` meta-argument referencing a single for_each instance
// (`a["x"].name`, config-known since count must be known at plan) must, like
// a body reference, depend on only that instance: `b` expands and creates
// without waiting for the delayed `a["y"]`. The recorded op order proves
// whether meta-argument references narrow.
func TestL2CountMetaInstanceRef(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_count_meta_instance_ref", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
