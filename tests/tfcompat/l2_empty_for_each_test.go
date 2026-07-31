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

// A `for_each` that evaluates to an empty set, and a `count = 0` resource,
// must still register the resource address as an empty collection so that
// outputs referencing the resource produce `{}` / `[]` rather than
// "Unsupported attribute". This is the pattern aws-ia/vpc uses for
// `aws_route_table.tgw` / `aws_route_table.cwan` — both are gated on opt-in
// subnet types and unconditionally surfaced in module outputs.
func TestL2EmptyForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_empty_for_each", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
