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

package schema

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// TestGenerateModuleSchemaGolden parses the HCL in each testdata case and
// asserts the Pulumi package schema produced by GenerateModuleSchema followed by
// ToPulumiPackageSchema against a golden schema.json. Run with PULUMI_ACCEPT=1 to
// (re)generate the golden files.
//
// The component schema is derived solely from the module's variables and
// outputs; it does not consult provider schemas. The package and component
// identity (name, version, component name, module segment) is supplied by
// NewHCLProvider in production, so each case passes those values explicitly.
func TestGenerateModuleSchemaGolden(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name          string
		pkgName       string
		version       string
		componentName string
		module        string
	}{
		{name: "primitives", pkgName: "primitives", version: "1.2.3", componentName: "primitives", module: "index"},
		{name: "collections", pkgName: "collections", version: "0.0.0-dev", componentName: "widget", module: "infra"},
		{name: "sensitive", pkgName: "sensitive", version: "0.0.0-dev", componentName: "sensitive", module: "index"},
		{name: "required", pkgName: "required", version: "0.0.0-dev", componentName: "required", module: "index"},
		{name: "inference", pkgName: "inference", version: "0.0.0-dev", componentName: "inference", module: "index"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			caseDir := filepath.Join("testdata", tc.name)
			config, diags := parser.NewParser().ParseDirectory(caseDir)
			require.False(t, diags.HasErrors(), diags.Error())

			moduleSchema, err := GenerateModuleSchema(config, tc.pkgName, tc.version, tc.componentName, tc.module)
			require.NoError(t, err)

			pkgSpec := moduleSchema.ToPulumiPackageSchema()
			got, err := json.MarshalIndent(pkgSpec, "", "  ")
			require.NoError(t, err)
			got = append(got, '\n')

			// Bind the generated schema against the Pulumi package metaschema so
			// that an invalid spec (e.g. a constant default on a non-primitive
			// property) fails the test rather than only `pulumi schema check`.
			var spec pulumiSchema.PackageSpec
			require.NoError(t, json.Unmarshal(got, &spec))
			_, bindDiags, err := pulumiSchema.BindSpec(spec, errLoader{}, pulumiSchema.ValidationOptions{})
			require.NoError(t, err)
			require.False(t, bindDiags.HasErrors(), bindDiags.Error())

			goldenPath := filepath.Join(caseDir, "schema.json")
			if cmdutil.IsTruthy(os.Getenv("PULUMI_ACCEPT")) {
				require.NoError(t, os.WriteFile(goldenPath, got, 0o644))
				return
			}

			want, err := os.ReadFile(goldenPath)
			require.NoError(t, err, "golden file %s not found; run with PULUMI_ACCEPT=1 to generate", goldenPath)
			assert.Equal(t, string(want), string(got))
		})
	}
}

// errLoader is a schema.Loader that fails if asked to load any package. The
// component schemas under test reference no external packages, so binding never
// invokes it; supplying it keeps BindSpec from constructing a real plugin loader.
type errLoader struct{}

func (errLoader) LoadPackage(pkg string, version *semver.Version) (*pulumiSchema.Package, error) {
	return nil, assert.AnError
}

func (errLoader) LoadPackageV2(
	ctx context.Context, descriptor *pulumiSchema.PackageDescriptor,
) (*pulumiSchema.Package, error) {
	return nil, assert.AnError
}
