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

// TestL1VarTFVarParsing pins pulumi-hcl's string-sourced variable parsing
// (`-var` / TF_VAR_ / Pulumi config) to OpenTofu's: the declared type selects
// the parsing mode.
func TestL1VarTFVarParsing(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvar_parsing", tfcompat.Case{
		Config: map[string]string{
			"str":      `["a", "b"]`,
			"num":      "42",
			"flag":     "true",
			"tags":     `{ environment = "prod" }`,
			"items":    `["a", "b"]`,
			"untyped":  `["a", "b"]`,
			"anyTyped": `["a", "b"]`,
		},
	})
}
