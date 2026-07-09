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

package tfcompat

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestResolveStages pins the positional matching between disk-loaded file
// sets and Case.Stages metadata: a flat directory fans out to one stage per
// metadata entry (all sharing the same files), numbered stage dirs pair 1:1
// with metadata, and a count mismatch on numbered dirs is an error.
func TestResolveStages(t *testing.T) {
	t.Parallel()
	a := map[string]string{"main.tf": "a"}
	b := map[string]string{"main.tf": "b"}

	t.Run("flat default", func(t *testing.T) {
		t.Parallel()
		runs, err := resolveStages([]map[string]string{a}, false, nil)
		require.NoError(t, err)
		assert.Equal(t, []stageRun{{files: a}}, runs)
	})

	t.Run("flat fans out over metadata", func(t *testing.T) {
		t.Parallel()
		runs, err := resolveStages([]map[string]string{a}, false, []Stage{
			{Mode: StagePreview},
			{},
		})
		require.NoError(t, err)
		assert.Equal(t, []stageRun{
			{files: a, Stage: Stage{Mode: StagePreview}},
			{files: a},
		}, runs)
	})

	t.Run("numbered pairs positionally", func(t *testing.T) {
		t.Parallel()
		runs, err := resolveStages([]map[string]string{a, b}, true, []Stage{
			{},
			{ExpectErr: "boom"},
		})
		require.NoError(t, err)
		assert.Equal(t, []stageRun{
			{files: a},
			{files: b, Stage: Stage{ExpectErr: "boom"}},
		}, runs)
	})

	t.Run("numbered without metadata", func(t *testing.T) {
		t.Parallel()
		runs, err := resolveStages([]map[string]string{a, b}, true, nil)
		require.NoError(t, err)
		assert.Equal(t, []stageRun{{files: a}, {files: b}}, runs)
	})

	t.Run("numbered count mismatch", func(t *testing.T) {
		t.Parallel()
		_, err := resolveStages([]map[string]string{a, b}, true, []Stage{{}})
		require.EqualError(t, err, "case has 2 numbered stage dirs but Case.Stages has 1 entries")
	})
}

// TestLoadCaseFS_MultiFile checks that the directory loader picks up every
// regular file, keyed by path relative to the case directory.
func TestLoadCaseFS_MultiFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.tf"), []byte("a-content"), 0o600))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "b.tf"), []byte("b-content"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "mod"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "mod", "c.tf"), []byte("c-content"), 0o600))

	files, err := loadCaseFS(dir)
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"a.tf":                       "a-content",
		"b.tf":                       "b-content",
		filepath.Join("mod", "c.tf"): "c-content",
	}, files)
}

// TestLoadCaseFS_MissingDir asserts the loader returns an error when the
// directory does not exist, rather than silently returning an empty map.
func TestLoadCaseFS_MissingDir(t *testing.T) {
	t.Parallel()
	_, err := loadCaseFS(filepath.Join(t.TempDir(), "does-not-exist"))
	require.Error(t, err)
	assert.Contains(t, err.Error(), "case directory")
}

// TestLoadCaseFS_NotADir asserts the loader rejects a file path masquerading
// as a case directory.
func TestLoadCaseFS_NotADir(t *testing.T) {
	t.Parallel()
	path := filepath.Join(t.TempDir(), "a.tf")
	require.NoError(t, os.WriteFile(path, []byte("x"), 0o600))
	_, err := loadCaseFS(path)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "not a directory")
}

// TestLoadCaseFS_Empty asserts the loader rejects an empty directory.
func TestLoadCaseFS_Empty(t *testing.T) {
	t.Parallel()
	_, err := loadCaseFS(t.TempDir())
	require.Error(t, err)
	assert.Contains(t, err.Error(), "no files found")
}
