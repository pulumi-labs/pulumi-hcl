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

package modules

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func mustHost(t *testing.T, s string) svchost.Hostname {
	t.Helper()
	host, err := svchost.ForComparison(s)
	require.NoError(t, err)
	return host
}

func TestTFTokenCredentials(t *testing.T) {
	// A dotted host names its variable with underscores; a hyphenated host uses
	// double underscores. An empty value and a non-TF_TOKEN_ name contribute
	// nothing.
	t.Setenv("TF_TOKEN_app_terraform_io", "tok-dotted")
	t.Setenv("TF_TOKEN_my__registry_example_com", "tok-hyphen")
	t.Setenv("TF_TOKEN_blank_example_com", "")
	t.Setenv("NOT_A_TF_TOKEN", "ignored")

	src := tfTokenCredentials()

	for _, tt := range []struct {
		host string
		want svcauth.HostCredentials
	}{
		{"app.terraform.io", svcauth.HostCredentialsToken("tok-dotted")},
		{"my-registry.example.com", svcauth.HostCredentialsToken("tok-hyphen")},
		{"blank.example.com", nil},
		{"unset.example.com", nil},
	} {
		got, err := src.ForHost(t.Context(), mustHost(t, tt.host))
		require.NoError(t, err)
		assert.Equal(t, tt.want, got, "host %q", tt.host)
	}
}

func TestLoadTerraformCLIConfig(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cli.tfrc")
	require.NoError(t, os.WriteFile(path, []byte(`
host "registry.example.com" {
  services = {
    "modules.v1" = "https://registry.example.com/modules/v1/"
  }
}

credentials "app.terraform.io" {
  token = "from-credentials-block"
}

# Unrelated blocks are tolerated and ignored.
plugin_cache_dir = "/tmp/plugins"
`), 0o600))
	t.Setenv("TF_CLI_CONFIG_FILE", path)

	cfg := loadTerraformCLIConfig()
	require.NotNil(t, cfg)

	creds, err := cliConfigCredentials(cfg).ForHost(t.Context(), mustHost(t, "app.terraform.io"))
	require.NoError(t, err)
	assert.Equal(t, svcauth.HostCredentialsToken("from-credentials-block"), creds)

	d := disco.New()
	applyHostServiceOverrides(d, cfg)
	u, err := d.DiscoverServiceURL(t.Context(), mustHost(t, "registry.example.com"), "modules.v1")
	require.NoError(t, err)
	assert.Equal(t, "https://registry.example.com/modules/v1/", u.String())
}

func TestLoadTerraformCLIConfigParseError(t *testing.T) {
	path := filepath.Join(t.TempDir(), "bad.tfrc")
	require.NoError(t, os.WriteFile(path, []byte(`host "unterminated" {`), 0o600))
	t.Setenv("TF_CLI_CONFIG_FILE", path)

	// A malformed file degrades to no config (the warning goes to stderr).
	assert.Nil(t, loadTerraformCLIConfig())
}

func TestLoadTerraformCLIConfigMissingExplicitFile(t *testing.T) {
	t.Setenv("TF_CLI_CONFIG_FILE", filepath.Join(t.TempDir(), "does-not-exist.tfrc"))
	assert.Nil(t, loadTerraformCLIConfig())
}
