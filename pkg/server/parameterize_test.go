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
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
)

func TestDefaultPackageName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		source string
		want   string
	}{
		{"terraform-aws-modules/vpc/aws", "vpc"},
		{"terraform-aws-modules/vpc/aws//modules/subnets", "vpc"},
		{"github.com/org/my-repo", "my-repo"},
		{"git::https://github.com/org/repo.git//subdir", "repo"},
		{"git::https://example.com/foo/Bar_Baz.git", "bar-baz"},
		{"./local/path", "path"},
	}
	for _, tc := range cases {
		t.Run(tc.source, func(t *testing.T) {
			t.Parallel()
			require.Equal(t, tc.want, defaultPackageName(tc.source))
		})
	}
}

func TestParameterizeArgsRejected(t *testing.T) {
	t.Parallel()

	t.Run("before handshake", func(t *testing.T) {
		t.Parallel()
		_, err := (&moduleProvider{}).parameterizeArgs(t.Context(), []string{"acme/widget/aws"})
		require.EqualError(t, err, "parameterize called before a successful handshake")
	})

	t.Run("no args", func(t *testing.T) {
		t.Parallel()
		_, err := (&moduleProvider{resolver: stubResolver{}}).parameterizeArgs(t.Context(), nil)
		require.EqualError(t, err, "the hcl provider is parameterized by a module source and an "+
			"optional version constraint: expected 1 or 2 arguments, got 0")
	})

	t.Run("too many args", func(t *testing.T) {
		t.Parallel()
		_, err := (&moduleProvider{resolver: stubResolver{}}).parameterizeArgs(t.Context(), []string{"a", "b", "c"})
		require.EqualError(t, err, "the hcl provider is parameterized by a module source and an "+
			"optional version constraint: expected 1 or 2 arguments, got 3")
	})
}

// TestParameterizeArgsServesTypedSchema drives the args path end-to-end for a
// provider-free local module: it must parameterize without touching the resolver
// and then serve the module's typed component schema (not the generic
// hcl:index:Module) with its parameterization Value attached.
func TestParameterizeArgsServesTypedSchema(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(filepath.Join("testdata", "module-one-var"))
	require.NoError(t, err)

	m := &moduleProvider{version: "1.2.3", resolver: stubResolver{}}
	resp, err := m.parameterizeArgs(t.Context(), []string{dir})
	require.NoError(t, err)
	require.Equal(t, "module-one-var", resp.Name)
	require.NotNil(t, m.param)

	out, err := m.getSchema(t.Context(), p.GetSchemaRequest{})
	require.NoError(t, err)

	var spec pulumiSchema.PackageSpec
	require.NoError(t, json.Unmarshal([]byte(out.Schema), &spec))

	require.NotNil(t, spec.Parameterization)
	require.Equal(t, "hcl", spec.Parameterization.BaseProvider.Name)

	res, ok := spec.Resources["module-one-var:index:module-one-var"]
	require.True(t, ok, "schema should declare the typed component")
	require.True(t, res.IsComponent)
	require.Equal(t, "string", res.InputProperties["name"].Type)
	require.Equal(t, "string", res.Properties["greeting"].Type)
	// "name" declares no `nullable = false`, so it is optional, matching the
	// MLC's handling of a nullable variable.
	require.Empty(t, res.RequiredInputs)
}

// TestBundleRoundTripResolvesOffline records a module tree — including a
// non-local submodule and an absolute-path root — bundles it, then resolves
// every source from the unpacked bundle with a resolver that has no fallback, so
// any unrecorded reference would error.
func TestBundleRoundTripResolvesOffline(t *testing.T) {
	t.Parallel()

	child := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(child, "main.tf"), []byte(`output "id" { value = "child" }`), 0o600))

	root := t.TempDir()
	require.NoError(t, os.WriteFile(
		filepath.Join(root, "main.tf"), []byte(`module "c" { source = "acme/widget/aws" }`), 0o600))

	// A custom resolver stands in for the network: the absolute-path root
	// resolves to itself and the submodule to the child package.
	rec := newResolveRecorder(func(packageSource, _, _ string) (string, error) {
		switch packageSource {
		case root:
			return root, nil
		case "acme/widget/aws":
			return child, nil
		default:
			return "", fmt.Errorf("unexpected source %q", packageSource)
		}
	})
	loader := modules.NewLoader(rec.resolve)

	// Loading the tree records every resolution into the recorder.
	loaded, err := loader.LoadModule(root, "", ".")
	require.NoError(t, err)
	_, err = loader.LoadModule("acme/widget/aws", "", loaded.SourcePath)
	require.NoError(t, err)

	value, err := rec.archive(moduleRef{Source: root})
	require.NoError(t, err)

	dest := t.TempDir()
	require.NoError(t, unpackArchive(value, dest))
	manifest, err := readManifest(dest)
	require.NoError(t, err)

	// The bundle resolver has no network and no filesystem fallback: every
	// source — the absolute-path root included — must come from the manifest.
	offline := modules.NewLoader(bundleResolver(dest, manifest))
	rootAgain, err := offline.LoadModule(manifest.Root.Source, manifest.Root.Version, ".")
	require.NoError(t, err)
	require.Contains(t, rootAgain.Config.Modules, "c")
	require.True(t, strings.HasPrefix(rootAgain.SourcePath, dest),
		"root must resolve inside the unpacked bundle, got %q", rootAgain.SourcePath)

	childAgain, err := offline.LoadModule("acme/widget/aws", "", rootAgain.SourcePath)
	require.NoError(t, err)
	require.Contains(t, childAgain.Config.Outputs, "id")
	require.True(t, strings.HasPrefix(childAgain.SourcePath, dest),
		"submodule must resolve inside the unpacked bundle, got %q", childAgain.SourcePath)
}

func TestPackArchiveDeterministic(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "a.tf"), []byte("a"), 0o600))
	require.NoError(t, os.MkdirAll(filepath.Join(dir, "sub"), 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(dir, "sub", "b.tf"), []byte("b"), 0o600))

	first, err := packArchive(map[string]string{"root": dir}, []byte(`{"root":{"source":"x"}}`))
	require.NoError(t, err)
	second, err := packArchive(map[string]string{"root": dir}, []byte(`{"root":{"source":"x"}}`))
	require.NoError(t, err)
	require.Equal(t, first, second)
}
