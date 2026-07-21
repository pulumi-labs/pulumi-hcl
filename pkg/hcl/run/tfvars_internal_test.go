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

package run

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// assertTfvars compares loaded values with cty's own equality: assert.Equal
// compares big.Float precision, which differs between a parsed number and a
// constructed one.
func assertTfvars(t *testing.T, expected, actual map[string]cty.Value) {
	t.Helper()
	want, got := cty.ObjectVal(expected), cty.ObjectVal(actual)
	assert.True(t, want.RawEquals(got), "expected %#v, got %#v", want, got)
}

func writeTfvarsDir(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return dir
}

func TestLoadTfvars(t *testing.T) {
	t.Parallel()

	t.Run("no files", func(t *testing.T) {
		t.Parallel()
		values, err := loadTfvars(t.TempDir())
		require.NoError(t, err)
		assert.Empty(t, values)
	})

	t.Run("types", func(t *testing.T) {
		t.Parallel()
		dir := writeTfvarsDir(t, map[string]string{
			"terraform.tfvars": `
s = "str"
n = 3
l = ["a", "b"]
o = { k = "v" }
`,
		})
		values, err := loadTfvars(dir)
		require.NoError(t, err)
		assertTfvars(t, map[string]cty.Value{
			"s": cty.StringVal("str"),
			"n": cty.NumberIntVal(3),
			"l": cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			"o": cty.ObjectVal(map[string]cty.Value{"k": cty.StringVal("v")}),
		}, values)
	})

	// Every file is loaded, and later files in the order terraform.tfvars,
	// terraform.tfvars.json, then *.auto.tfvars* lexically win over earlier ones.
	t.Run("precedence", func(t *testing.T) {
		t.Parallel()
		dir := writeTfvarsDir(t, map[string]string{
			"terraform.tfvars":      `winner = "tfvars"` + "\n" + `from_tfvars = "yes"`,
			"terraform.tfvars.json": `{"winner": "json", "from_json": "yes"}`,
			"a.auto.tfvars":         `winner = "a-auto"` + "\n" + `from_auto = "yes"`,
			"b.auto.tfvars.json":    `{"winner": "b-auto-json"}`,
			"ignored.tfvars":        `winner = "ignored"`,
		})
		values, err := loadTfvars(dir)
		require.NoError(t, err)
		assertTfvars(t, map[string]cty.Value{
			"winner":      cty.StringVal("b-auto-json"),
			"from_tfvars": cty.StringVal("yes"),
			"from_json":   cty.StringVal("yes"),
			"from_auto":   cty.StringVal("yes"),
		}, values)
	})

	t.Run("malformed", func(t *testing.T) {
		t.Parallel()
		dir := writeTfvarsDir(t, map[string]string{"terraform.tfvars": `x = `})
		_, err := loadTfvars(dir)
		require.ErrorContains(t, err, "reading terraform.tfvars:")
	})

	t.Run("references are not allowed", func(t *testing.T) {
		t.Parallel()
		dir := writeTfvarsDir(t, map[string]string{"terraform.tfvars": `x = var.y`})
		_, err := loadTfvars(dir)
		require.ErrorContains(t, err, "Variables not allowed")
	})
}
