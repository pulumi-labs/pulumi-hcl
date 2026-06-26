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
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"
)

// tfTokenCredentials reads TF_TOKEN_<host> environment variables into a
// credentials source, the way Terraform and OpenTofu authenticate to a
// service-discovery host with no `terraform login`. The host portion of the
// variable name is decoded the same way they decode it: "__" becomes "-" and
// "_" becomes ".", so a host with hyphens or dots can be named with a
// shell-legal variable. An invalid host or empty value is skipped.
func tfTokenCredentials() svcauth.CredentialsSource {
	const prefix = "TF_TOKEN_"
	creds := map[svchost.Hostname]svcauth.HostCredentials{}
	for _, ev := range os.Environ() {
		name, value, ok := strings.Cut(ev, "=")
		if !ok || value == "" || !strings.HasPrefix(name, prefix) {
			continue
		}
		rawHost := name[len(prefix):]
		rawHost = strings.ReplaceAll(rawHost, "__", "-")
		rawHost = strings.ReplaceAll(rawHost, "_", ".")
		host, err := svchost.ForComparison(svchost.ForDisplay(rawHost))
		if err != nil {
			continue
		}
		creds[host] = svcauth.HostCredentialsToken(value)
	}
	return svcauth.StaticCredentialsSource(creds)
}

// terraformCLIConfig holds the parts of a Terraform/OpenTofu CLI configuration
// file that affect module registry resolution: `host` blocks that override
// service discovery and `credentials` blocks that authenticate to a host.
// Everything else in the file is absorbed by the trailing remain bodies and
// ignored.
type terraformCLIConfig struct {
	Hosts       []cliConfigHost  `hcl:"host,block"`
	Credentials []cliConfigCreds `hcl:"credentials,block"`
	Remain      hcl.Body         `hcl:",remain"`
}

type cliConfigHost struct {
	Name     string            `hcl:"name,label"`
	Services map[string]string `hcl:"services,optional"`
	Remain   hcl.Body          `hcl:",remain"`
}

type cliConfigCreds struct {
	Name   string   `hcl:"name,label"`
	Token  string   `hcl:"token,optional"`
	Remain hcl.Body `hcl:",remain"`
}

// terraformCLIConfigPath resolves the CLI configuration file the same way
// OpenTofu does: an explicit TF_CLI_CONFIG_FILE (or the legacy TERRAFORM_CONFIG)
// wins, otherwise the per-user default. explicit reports whether the path came
// from one of those environment variables, so a missing explicit file warns
// while a missing default file stays silent.
func terraformCLIConfigPath() (path string, explicit bool) {
	if p := os.Getenv("TF_CLI_CONFIG_FILE"); p != "" {
		return p, true
	}
	if p := os.Getenv("TERRAFORM_CONFIG"); p != "" {
		return p, true
	}
	if runtime.GOOS == "windows" {
		if appData := os.Getenv("APPDATA"); appData != "" {
			return filepath.Join(appData, "terraform.rc"), false
		}
		return "", false
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", false
	}
	return filepath.Join(home, ".terraformrc"), false
}

// loadTerraformCLIConfig reads and parses the CLI configuration file. A missing
// default file is not an error and returns nil. Any other failure — an
// unreadable explicit file, or a file that does not parse — is reported loudly
// to stderr and returns nil, so a malformed config degrades to "no overrides,
// no extra credentials" rather than breaking the run.
func loadTerraformCLIConfig() *terraformCLIConfig {
	path, explicit := terraformCLIConfigPath()
	if path == "" {
		return nil
	}

	src, err := os.ReadFile(path)
	if err != nil {
		if explicit || !os.IsNotExist(err) {
			warnTerraformCLIConfig("could not read Terraform CLI configuration file %q: %v", path, err)
		}
		return nil
	}

	file, diags := hclparse.NewParser().ParseHCL(src, path)
	if diags.HasErrors() {
		warnTerraformCLIConfig("could not parse Terraform CLI configuration file %q: %s", path, diags)
		return nil
	}

	var cfg terraformCLIConfig
	if diags := gohcl.DecodeBody(file.Body, nil, &cfg); diags.HasErrors() {
		warnTerraformCLIConfig("could not decode Terraform CLI configuration file %q: %s", path, diags)
		return nil
	}
	return &cfg
}

// cliConfigCredentials turns the file's `credentials` blocks into a credentials
// source. A nil config or a config with no credentials yields an empty source
// that authenticates nothing.
func cliConfigCredentials(cfg *terraformCLIConfig) svcauth.CredentialsSource {
	creds := map[svchost.Hostname]svcauth.HostCredentials{}
	if cfg != nil {
		for _, c := range cfg.Credentials {
			if c.Token == "" {
				continue
			}
			host, err := svchost.ForComparison(svchost.ForDisplay(c.Name))
			if err != nil {
				warnTerraformCLIConfig("ignoring credentials block for invalid host %q: %v", c.Name, err)
				continue
			}
			creds[host] = svcauth.HostCredentialsToken(c.Token)
		}
	}
	return svcauth.StaticCredentialsSource(creds)
}

// applyHostServiceOverrides registers each `host` block's service map with the
// discovery client, so a request for that host uses the configured service URLs
// instead of network discovery — the standard way to point a registry host at a
// mirror, proxy, or (in tests) a local server.
func applyHostServiceOverrides(d *disco.Disco, cfg *terraformCLIConfig) {
	if cfg == nil {
		return
	}
	for _, h := range cfg.Hosts {
		host, err := svchost.ForComparison(svchost.ForDisplay(h.Name))
		if err != nil {
			warnTerraformCLIConfig("ignoring host block for invalid host %q: %v", h.Name, err)
			continue
		}
		services := make(map[string]any, len(h.Services))
		for service, url := range h.Services {
			services[service] = url
		}
		d.ForceHostServices(host, services)
	}
}

// warnTerraformCLIConfig writes a prominent warning to stderr. The language
// host's stderr is surfaced to the user by the engine, so a misconfigured CLI
// file is visible rather than silently dropped.
func warnTerraformCLIConfig(format string, args ...any) {
	fmt.Fprintf(os.Stderr, "warning: "+format+"\n", args...)
}
