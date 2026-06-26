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

// TestL2ModuleSensitiveInput demonstrates that the sensitive mark flows through
// an in-language module in both directions: a sensitive variable passed as a
// module input reaches the resource inside the module still marked sensitive
// (inner_is_sensitive), and a value derived from that input inside the module
// carries the mark back out to the root (outer_is_sensitive). The actual value
// is asserted to round-trip unchanged via connection_value.
func TestL2ModuleSensitiveInput(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_sensitive_input", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
