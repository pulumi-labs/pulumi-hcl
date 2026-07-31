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

// TestL2ModuleSensitiveOutput asserts that a child-module output declared
// `sensitive = true` carries its sensitive mark into the calling module. The
// root reads it through `issensitive`, which OpenTofu reports as true; pulumi-hcl
// does not apply the declared mark and reports false.
func TestL2ModuleSensitiveOutput(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_sensitive_output", tfcompat.Case{})
}
