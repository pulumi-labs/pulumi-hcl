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

// A `for_each` key containing a `[` character. OpenTofu treats the key as an
// opaque string and creates one instance per key. pulumi-hcl derives the
// per-instance Pulumi resource name by re-parsing the instance address and
// scanning for `[` from the end, which misreads a bracket inside the key. Both
// keys collapse to the bare logical name `web`, so the second instance fails
// with "Duplicate resource URN".
func TestL2ForEachBracketKey(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_for_each_bracket_key", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
