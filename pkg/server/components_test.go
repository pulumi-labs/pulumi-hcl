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

package server

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi-hcl/vendored/getmodules"
	"github.com/stretchr/testify/require"
)

// writeModuleTree writes empty .tf files at the given root-relative paths and
// returns the root directory.
func writeModuleTree(t *testing.T, paths ...string) string {
	t.Helper()
	root := t.TempDir()
	for _, p := range paths {
		full := filepath.Join(root, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, nil, 0o600))
	}
	return root
}

func TestDiscoverComponentDirs(t *testing.T) {
	t.Parallel()

	t.Run("root only", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "main.tf")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{{}}, got)
	})

	t.Run("root and submodules", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "main.tf", "modules/alpha/main.tf", "modules/beta/main.tf")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{
			{},
			{Subdir: "modules/alpha"},
			{Subdir: "modules/beta"},
		}, got)
	})

	t.Run("submodules only", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "modules/alpha/main.tf")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{{Subdir: "modules/alpha"}}, got)
	})

	t.Run("sanitizes directory names", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "modules/_user_data/main.tf", "modules/a.b/main.tf")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{{Subdir: "modules/_user_data"}, {Subdir: "modules/a.b"}}, got)
		require.Equal(t, "user-data", got[0].module())
		require.Equal(t, "a-b", got[1].module())
	})

	t.Run("skips non-module directories", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "main.tf", "modules/docs/README.md", "modules/.hidden/main.tf")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{{}}, got)
	})

	t.Run("no components", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "README.md")
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Empty(t, got)
	})

	t.Run("colliding names", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "modules/user-data/main.tf", "modules/user_data/main.tf")
		_, err := discoverComponentDirs(root)
		require.EqualError(t, err,
			`submodule directories "user-data" and "user_data" both map to module name "user-data"`)
	})

	t.Run("skips symlinked files and directories", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "main.tf")
		require.NoError(t, os.MkdirAll(filepath.Join(root, "modules", "symfile"), 0o755))
		require.NoError(t, os.Symlink(filepath.Join(root, "main.tf"),
			filepath.Join(root, "modules", "symfile", "main.tf")))
		real := writeModuleTree(t, "main.tf")
		require.NoError(t, os.Symlink(real, filepath.Join(root, "modules", "symdir")))
		got, err := discoverComponentDirs(root)
		require.NoError(t, err)
		require.Equal(t, []componentDir{{}}, got)
	})

	t.Run("unusable name", func(t *testing.T) {
		t.Parallel()
		root := writeModuleTree(t, "modules/---/main.tf")
		_, err := discoverComponentDirs(root)
		require.EqualError(t, err, `submodule directory "---" does not sanitize to a usable module name`)
	})
}

func TestComponentSource(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name   string
		source string
		dir    componentDir
		want   string
	}{
		{"root", "acme/widget/cloud", componentDir{}, "acme/widget/cloud"},
		{
			"registry submodule",
			"acme/widget/cloud",
			componentDir{Subdir: "modules/alpha"},
			"acme/widget/cloud//modules/alpha",
		},
		{
			"source with existing subdir",
			"git::https://example.com/repo.git//nested",
			componentDir{Subdir: "modules/alpha"},
			"git::https://example.com/repo.git//nested/modules/alpha",
		},
		{
			"registry with version query",
			"acme/widget/cloud?version=1.2.3",
			componentDir{Subdir: "modules/alpha"},
			"acme/widget/cloud//modules/alpha?version=1.2.3",
		},
		{
			"git with ref query",
			"git::https://example.com/repo.git?ref=v1",
			componentDir{Subdir: "modules/alpha"},
			"git::https://example.com/repo.git//modules/alpha?ref=v1",
		},
		{
			"subdir and query",
			"https://example.com/x.zip//sub?ref=y",
			componentDir{Subdir: "modules/alpha"},
			"https://example.com/x.zip//sub/modules/alpha?ref=y",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, componentSource(tc.source, tc.dir))
			if tc.dir.Subdir != "" {
				_, subdir := getmodules.SplitPackageSubdir(tc.want)
				require.NotEmpty(t, subdir, "the produced source must re-split into a subdir")
			}
		})
	}
}
