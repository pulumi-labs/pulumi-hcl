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

// An ignore_changes traversal into terraform_data's input — a map key or a
// numeric list index — ignores the whole attribute: input is a single dynamic
// attribute, so the prior input (all of it, not just the ignored key) is kept
// across an apply that changes it.
func TestL2TdataIgnoreInputIndex(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_input_index", tfcompat.Case{})
}

func TestL2TdataIgnoreInputNum(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_tdata_ignore_input_num", tfcompat.Case{})
}
