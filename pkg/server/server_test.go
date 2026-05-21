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

	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Test that we correctly inline additional keys.
func TestGenerateProjectInlinesAdditionalKeys(t *testing.T) {
	t.Parallel()

	test := func(t *testing.T, projectJSON, expectedYAML string) {

		sourceDir := t.TempDir()
		require.NoError(t, os.WriteFile(filepath.Join(sourceDir, "main.pp"),
			[]byte("output hello {\n    value = \"world\"\n}\n"), 0o600))

		targetDir := t.TempDir()

		host := &LanguageHost{}
		_, err := host.GenerateProject(t.Context(), &pulumirpc.GenerateProjectRequest{
			SourceDirectory: sourceDir,
			TargetDirectory: targetDir,
			Project:         projectJSON,
			LoaderTarget:    "127.0.0.1:1",
		})
		require.NoError(t, err)

		data, err := os.ReadFile(filepath.Join(targetDir, "Pulumi.yaml"))
		require.NoError(t, err)

		require.Equal(t, expectedYAML, string(data))
	}

	t.Run("no additional keys", func(t *testing.T) {
		t.Parallel()

		json := `{
        "name": "test",
        "description": "test project"
    }`

		yaml := `name: test
runtime: hcl
description: test project
`

		test(t, json, yaml)
	})

	t.Run("with additional keys", func(t *testing.T) {
		t.Parallel()

		json := `{
        "name": "test",
        "description": "test project",
	"AdditionalKeys": { "fizz": "buzz" }
    }`

		yaml := `name: test
runtime: hcl
description: test project
fizz: buzz
`

		test(t, json, yaml)
	})
}

// TestGeneratePackageAndRunUseSameSdksDir locks in the contract that the
// language writes parameterization info to <projectDir>/sdks/<name>/hcl.sdk.json
// (where `pulumi package add` puts it) and that GetRequiredPackages reads it
// from the same place. Conformance tests previously masked a mismatch where
// GeneratePackage wrote to sdks/ but the runtime read from .hcl/sdks/.
func TestGeneratePackageAndRunUseSameSdksDir(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()

	// Mirror `pulumi package add`: caller creates sdks/<name>/ then calls
	// GeneratePackage with that directory.
	const alias = "myparam"
	sdkDir := filepath.Join(projectDir, "sdks", alias)
	require.NoError(t, os.MkdirAll(sdkDir, 0o755))

	schema := `{
		"name": "myparam",
		"version": "1.2.3",
		"parameterization": {
			"baseProvider": {
				"name": "baseplugin",
				"version": "1.0.0"
			},
			"parameter": "aGVsbG8="
		}
	}`

	host := &LanguageHost{}
	_, err := host.GeneratePackage(t.Context(), &pulumirpc.GeneratePackageRequest{
		Directory: sdkDir,
		Schema:    schema,
	})
	require.NoError(t, err)

	// Lock in the path: GeneratePackage must write here, not under .hcl/sdks.
	_, err = os.Stat(filepath.Join(projectDir, "sdks", alias, "hcl.sdk.json"))
	require.NoError(t, err, "GeneratePackage must write hcl.sdk.json to sdks/<name>/")
	_, err = os.Stat(filepath.Join(projectDir, ".hcl", "sdks", alias, "hcl.sdk.json"))
	require.True(t, os.IsNotExist(err), "hcl.sdk.json must not be written under .hcl/sdks/")

	// Write an HCL program that references the alias.
	program := `pulumi {
  required_providers {
    myparam = {
      source  = "myparam"
      version = "1.2.3"
    }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.hcl"), []byte(program), 0o600))

	resp, err := host.GetRequiredPackages(t.Context(), &pulumirpc.GetRequiredPackagesRequest{
		Info: &pulumirpc.ProgramInfo{
			ProgramDirectory: projectDir,
			RootDirectory:    projectDir,
			EntryPoint:       ".",
		},
	})
	require.NoError(t, err)

	require.Len(t, resp.Packages, 1)
	assert.Equal(t, &pulumirpc.PackageDependency{
		Name:    "baseplugin",
		Version: "1.0.0",
		Kind:    "resource",
		Parameterization: &pulumirpc.PackageParameterization{
			Name:    "myparam",
			Version: "1.2.3",
			Value:   []byte("hello"),
		},
	}, resp.Packages[0])
}
