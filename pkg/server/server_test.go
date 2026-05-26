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

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
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
	program := `terraform {
  required_providers {
    myparam = {
      source  = "myparam"
      version = "1.2.3"
    }
  }
}
`
	require.NoError(t, os.WriteFile(filepath.Join(projectDir, "main.tf"), []byte(program), 0o600))

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

// TestMissingNonPulumiSDKs_ImplicitProvider reproduces the tf_stack_test bug:
// a program references `data "archive_file" ...` without declaring `archive`
// in required_providers. The provider is *implicit* — its only mention is in
// the data source's type prefix. The previous implementation only looked at
// required_providers, so the missing SDK slipped through and Run sent the
// engine a raw "registry.terraform.io/hashicorp/archive" provider request.
// missingNonPulumiSDKs must catch the implicit provider too.
func TestMissingNonPulumiSDKs_ImplicitProvider(t *testing.T) {
	t.Parallel()

	const src = `terraform {
  required_providers {
    aws = {
      source  = "hashicorp/aws"
      version = "6.19.0"
    }
  }
}

resource "aws_s3_bucket" "b" {}

data "archive_file" "lambda" {}
`
	cfg, diags := parser.NewParser().ParseSource("main.tf", []byte(src))
	require.False(t, diags.HasErrors(), "diags: %v", diags)

	// No SDKs on disk: both non-Pulumi providers (explicit aws, implicit
	// archive) must be reported missing.
	assert.Equal(t,
		[]string{"archive", "aws"},
		missingNonPulumiSDKs(cfg, nil, ""))

	// Once both have SDKs, nothing is missing.
	sdks := map[string]workspace.PackageDescriptor{
		"aws":     {PluginDescriptor: workspace.PluginDescriptor{Name: "aws"}},
		"archive": {PluginDescriptor: workspace.PluginDescriptor{Name: "archive"}},
	}
	assert.Empty(t, missingNonPulumiSDKs(cfg, sdks, ""))

	// A pulumi-source provider needs no SDK even when it's only referenced
	// by a resource type prefix.
	const pulumiSrc = `terraform {
  required_providers {
    aws = {
      source  = "pulumi/aws"
      version = "6.0.0"
    }
  }
}

resource "aws_s3_bucket" "b" {}
`
	cfgPulumi, diags := parser.NewParser().ParseSource("main.tf", []byte(pulumiSrc))
	require.False(t, diags.HasErrors(), "diags: %v", diags)
	assert.Empty(t, missingNonPulumiSDKs(cfgPulumi, nil, ""))
}

// Implicit provider inside a child module must surface at the top — without
// recursion the SDK check silently misses it (the aws-ia/label gap).
func TestMissingNonPulumiSDKs_TransitiveModuleProvider(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
module "child" {
  source = "./child"
}
`), 0o600))

	childDir := filepath.Join(dir, "child")
	require.NoError(t, os.MkdirAll(childDir, 0o755))
	// Only mention of "aws" is inside the module.
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "main.tf"), []byte(`
resource "aws_s3_bucket" "b" {}
`), 0o600))

	cfg, diags := parser.NewParser().ParseDirectory(dir)
	require.False(t, diags.HasErrors(), "diags: %v", diags)

	assert.Equal(t,
		[]string{"aws"},
		missingNonPulumiSDKs(cfg, nil, dir))
}

// Same recursion via the module's `required_providers` block (no resources).
func TestMissingNonPulumiSDKs_TransitiveModuleRequiredProviders(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(`
module "child" {
  source = "./child"
}
`), 0o600))

	childDir := filepath.Join(dir, "child")
	require.NoError(t, os.MkdirAll(childDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(childDir, "main.tf"), []byte(`
terraform {
  required_providers {
    awscc = {
      source  = "hashicorp/awscc"
      version = ">= 1.0"
    }
  }
}
`), 0o600))

	cfg, diags := parser.NewParser().ParseDirectory(dir)
	require.False(t, diags.HasErrors(), "diags: %v", diags)

	assert.Equal(t,
		[]string{"awscc"},
		missingNonPulumiSDKs(cfg, nil, dir))
}
