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
)

// An outer module call with `count = 0` must skip all of its inner content —
// nested modules, locals, resources — instead of falling back to the root
// eval context. Reproduces the failure observed in aws-ia/rds-aurora where
// the secondary VPC (gated on `var.setup_globaldb`) is skipped but its
// internal `subnet_tags` submodule still ran and could not find the outer
// module's `local.subnet_keys_with_tags`.
func TestL2ZeroCountParentModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_zero_count_parent_module", tfcompat.Case{})
}
