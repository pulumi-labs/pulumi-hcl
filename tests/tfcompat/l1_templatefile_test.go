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

// TestL1TemplateFile pins that `templatefile` renders its file as a full HCL
// template — interpolations may call functions and use `%{ for }` / `%{ if }`
// directives — exactly as OpenTofu does, rather than performing a naive
// `${name}` string substitution that leaves function calls and directives
// untouched.
func TestL1TemplateFile(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_templatefile", tfcompat.Case{})
}
