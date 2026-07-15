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

// A for_each key that contains a "-" can make one resource's Pulumi URN name
// collide with another resource's. OpenTofu addresses instances by their
// typed (resource, key) pair, so `simple_resource.r["a-b"]` and
// `simple_resource.r-a["b"]` are distinct and apply cleanly. pulumi-hcl
// derives the URN name by joining the logical name and the for_each key with
// "-", so both collapse to "r-a-b" and `pulumi up` fails with a duplicate URN.
func TestL2ForEachKeyURNCollision(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_for_each_key_urn_collision", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
