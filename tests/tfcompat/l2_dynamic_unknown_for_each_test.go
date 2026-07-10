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

// A `dynamic` block whose `for_each` reads a computed attribute of a
// not-yet-created resource is unknown at preview time. Preview must
// treat the whole block property as unknown instead of panicking with
// "can't use ElementIterator on unknown value"; apply then expands the
// block with the known value.
func TestL2DynamicUnknownForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_dynamic_unknown_for_each", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
			{Name: "blocky", Factory: providers.BlockyProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{Mode: tfcompat.StageApply},
		},
	})
}
