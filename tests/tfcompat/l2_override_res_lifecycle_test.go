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

// TestL2OverrideResLifecycle checks that an override file's lifecycle block
// amends the base lifecycle rather than replacing it: the override sets only
// create_before_destroy, so the base's ignore_changes still holds and stage 1
// keeps the value applied in stage 0.
func TestL2OverrideResLifecycle(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_override_res_lifecycle", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
