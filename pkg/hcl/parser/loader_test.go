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

package parser

import (
	"os"
	"path/filepath"
	"sort"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadDirectorySkipsIgnoredFiles(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	for name, content := range map[string]string{
		"main.tf":           "",
		"resources.tf.json": "{}",
		".hidden.tf":        "this would not parse",
		"main.tf~":          "this would not parse",
		"#main.tf#":         "this would not parse",
	} {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}

	files, diags := NewLoader().LoadDirectory(dir)
	require.False(t, diags.HasErrors(), diags.Error())

	loaded := make([]string, 0, len(files))
	for path := range files {
		loaded = append(loaded, filepath.Base(path))
	}
	sort.Strings(loaded)

	assert.Equal(t, []string{"main.tf", "resources.tf.json"}, loaded)
}

func TestIsIgnoredFile(t *testing.T) {
	t.Parallel()

	assert.True(t, isIgnoredFile(".hidden.tf"))
	assert.True(t, isIgnoredFile("main.tf~"))
	assert.True(t, isIgnoredFile("#main.tf#"))
	assert.False(t, isIgnoredFile("main.tf"))
	assert.False(t, isIgnoredFile("resources.tf.json"))
}
