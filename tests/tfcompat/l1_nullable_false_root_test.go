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

// TestL1NullableFalseRoot assigns the null literal to a root variable declared
// with `nullable = false` and a default. OpenTofu substitutes the default (so
// `var.v == null` is false and `var.v` is the optional-filled object);
// pulumi-hcl leaves the value null at the root scope, diverging. The module
// input path already substitutes (see TestL1NullableFalseDefault); only the
// root variable path is affected.
func TestL1NullableFalseRoot(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_nullable_false_root", tfcompat.Case{
		Config: map[string]string{"v": "null"},
	})
}
