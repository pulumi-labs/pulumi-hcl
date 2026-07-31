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

// A child module declares `items` with `nullable = false` and a default. The
// caller passes `items = null`, which Terraform/OpenTofu replace with the
// variable's default, so `length(var.items)` evaluates to 2. Both paths must
// agree. See https://github.com/pulumi/pulumi-hcl/issues/183.
func TestL1NullableFalseDefault(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_nullable_false_default", tfcompat.Case{})
}
