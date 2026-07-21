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
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func TestIsOverrideFile(t *testing.T) {
	t.Parallel()

	assert.True(t, isOverrideFile("override.tf"))
	assert.True(t, isOverrideFile("override.tf.json"))
	assert.True(t, isOverrideFile("/modules/thing/db_override.tf"))
	assert.True(t, isOverrideFile("db_override.tf.json"))
	assert.False(t, isOverrideFile("main.tf"))
	assert.False(t, isOverrideFile("override_db.tf"))
	assert.False(t, isOverrideFile("dboverride.tf"))
}

// parseDir writes files into a temporary directory and parses it.
func parseDir(t *testing.T, files map[string]string) (*ast.Config, hcl.Diagnostics) {
	t.Helper()
	dir := t.TempDir()
	for name, content := range files {
		require.NoError(t, os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600))
	}
	return NewParser().ParseDirectory(dir)
}

// bodyValues evaluates every argument of body against the given schema.
func bodyValues(t *testing.T, body hcl.Body, names ...string) map[string]cty.Value {
	t.Helper()
	schema := &hcl.BodySchema{}
	for _, name := range names {
		schema.Attributes = append(schema.Attributes, hcl.AttributeSchema{Name: name})
	}
	content, _, diags := body.PartialContent(schema)
	require.False(t, diags.HasErrors(), diags.Error())

	values := map[string]cty.Value{}
	for name, attr := range content.Attributes {
		value, valDiags := attr.Expr.Value(nil)
		require.False(t, valDiags.HasErrors(), valDiags.Error())
		values[name] = value
	}
	return values
}

func TestOverrideResource(t *testing.T) {
	t.Parallel()

	config, diags := parseDir(t, map[string]string{
		"main.tf": `
resource "simple_resource" "r" {
  count     = 1
  input_one = "base"
  input_two = false
  nested { value = "base" }
}
`,
		"override.tf": `
resource "simple_resource" "r" {
  count     = 2
  input_one = "overridden"
  nested { value = "overridden" }
}
`,
	})
	require.False(t, diags.HasErrors(), diags.Error())

	resource := config.Resources[ast.ResourceKey("simple_resource", "r")]
	require.NotNil(t, resource)

	count, countDiags := resource.Count.Value(nil)
	require.False(t, countDiags.HasErrors(), countDiags.Error())
	assert.Equal(t, "2", count.AsBigFloat().String())

	assert.Equal(t, map[string]cty.Value{
		"input_one": cty.StringVal("overridden"),
		"input_two": cty.False,
	}, bodyValues(t, resource.Config, "input_one", "input_two"))

	// A block type the override declares hides every base block of that type.
	content, _, contentDiags := resource.Config.PartialContent(&hcl.BodySchema{
		Blocks: []hcl.BlockHeaderSchema{{Type: "nested"}},
	})
	require.False(t, contentDiags.HasErrors(), contentDiags.Error())
	require.Len(t, content.Blocks, 1)
	assert.Equal(t, map[string]cty.Value{"value": cty.StringVal("overridden")},
		bodyValues(t, content.Blocks[0].Body, "value"))
}

func TestOverrideMissingBaseDeclaration(t *testing.T) {
	t.Parallel()

	_, diags := parseDir(t, map[string]string{
		"main.tf": `resource "simple_resource" "r" { input_one = "base" }`,
		"override.tf": `
resource "simple_resource" "other" { input_one = "overridden" }
variable "nope" { default = 1 }
`,
	})

	summaries := make([]string, 0, len(diags))
	for _, diag := range diags {
		summaries = append(summaries, diag.Summary)
	}
	assert.Equal(t, []string{
		"Missing resource to override",
		"Missing variable to override",
	}, summaries)
}

func TestOverrideDependsOnRejected(t *testing.T) {
	t.Parallel()

	_, diags := parseDir(t, map[string]string{
		"main.tf":     `resource "simple_resource" "r" { input_one = "base" }`,
		"override.tf": `resource "simple_resource" "r" { depends_on = [simple_resource.other] }`,
	})

	require.Len(t, diags, 1)
	assert.Equal(t, "Unsupported override", diags[0].Summary)
	assert.Equal(t, "The depends_on argument may not be overridden.", diags[0].Detail)
}

func TestOverrideLocals(t *testing.T) {
	t.Parallel()

	config, diags := parseDir(t, map[string]string{
		"main.tf": `
locals {
  a = "base"
  b = "base"
}
`,
		"override.tf": `locals { a = "overridden" }`,
	})
	require.False(t, diags.HasErrors(), diags.Error())

	values := map[string]cty.Value{}
	for name, local := range config.Locals {
		value, valDiags := local.Value.Value(nil)
		require.False(t, valDiags.HasErrors(), valDiags.Error())
		values[name] = value
	}
	assert.Equal(t, map[string]cty.Value{
		"a": cty.StringVal("overridden"),
		"b": cty.StringVal("base"),
	}, values)
}

func TestOverrideMissingBaseLocal(t *testing.T) {
	t.Parallel()

	_, diags := parseDir(t, map[string]string{
		"main.tf":     `locals { a = "base" }`,
		"override.tf": `locals { c = "overridden" }`,
	})

	require.Len(t, diags, 1)
	assert.Equal(t, "Missing local value to override", diags[0].Summary)
}

func TestOverrideRequiredProviders(t *testing.T) {
	t.Parallel()

	config, diags := parseDir(t, map[string]string{
		"main.tf": `
terraform {
  required_providers {
    simple = { source = "pulumi/simple", version = "1.0.0" }
    other  = { source = "pulumi/other", version = "1.0.0" }
  }
}
`,
		"override.tf": `
terraform {
  required_providers {
    simple = { source = "pulumi/simple", version = "2.0.0" }
  }
}
`,
	})
	require.False(t, diags.HasErrors(), diags.Error())

	require.NotNil(t, config.Terraform)
	versions := map[string]string{}
	for name, required := range config.Terraform.RequiredProviders {
		versions[name] = required.Version
	}
	assert.Equal(t, map[string]string{"simple": "2.0.0", "other": "1.0.0"}, versions)
}

func TestOverrideProviderConfiguration(t *testing.T) {
	t.Parallel()

	config, diags := parseDir(t, map[string]string{
		"main.tf": `
provider "simple" { region = "base" }
provider "simple" {
  alias  = "west"
  region = "base"
}
`,
		"override.tf": `
provider "simple" { region = "overridden" }
provider "simple" {
  alias  = "west"
  region = "overridden"
}
`,
	})
	require.False(t, diags.HasErrors(), diags.Error())

	for _, key := range []string{"simple", "simple.west"} {
		provider := config.Providers[key]
		require.NotNil(t, provider, key)
		assert.Equal(t, map[string]cty.Value{"region": cty.StringVal("overridden")},
			bodyValues(t, provider.Config, "region"), key)
	}
}

func TestOverrideMissingAliasedProvider(t *testing.T) {
	t.Parallel()

	config, diags := parseDir(t, map[string]string{
		"main.tf": `resource "simple_resource" "r" { input_one = "base" }`,
		"override.tf": `
provider "simple" { region = "new" }
provider "simple" {
  alias  = "east"
  region = "new"
}
`,
	})

	// A default provider configuration may be introduced by an override file;
	// an aliased one may not.
	require.Len(t, diags, 1)
	assert.Equal(t, "Missing provider configuration to override", diags[0].Summary)
	assert.NotNil(t, config.Providers["simple"])
}

func TestOverrideMovedAndImportRejected(t *testing.T) {
	t.Parallel()

	_, diags := parseDir(t, map[string]string{
		"main.tf": `resource "simple_resource" "r" { input_one = "base" }`,
		"override.tf": `
moved {
  from = simple_resource.old
  to   = simple_resource.r
}
`,
	})

	require.Len(t, diags, 1)
	assert.Equal(t, `Cannot override "moved" blocks`, diags[0].Summary)
}
