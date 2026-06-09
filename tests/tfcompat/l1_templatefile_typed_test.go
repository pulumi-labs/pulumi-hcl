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

// TestL1TemplateFileTyped pins that `templatefile` returns the native type of a
// template that is a single interpolation, exactly as OpenTofu does. When the
// template is `${a}` and `a` is a list, OpenTofu's templatefile returns that
// list value (its return type is computed from the rendered template, not
// statically `string`). pulumi-hcl always coerces the rendered template to a
// string, so a non-string single-interpolation result diverges.
func TestL1TemplateFileTyped(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_templatefile_typed", tfcompat.Case{})
}
