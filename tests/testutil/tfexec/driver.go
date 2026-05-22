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

// Package tfexec drives the Terraform/OpenTofu CLI against in-process TF
// providers (via reattach) so tests can exercise real Terraform behavior
// without installing remote provider binaries.
package tfexec

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/go-plugin"
	"github.com/hashicorp/terraform-plugin-go/tfprotov5"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
	"github.com/hashicorp/terraform-plugin-go/tfprotov6/tf6server"
	"github.com/hashicorp/terraform-plugin-mux/tf5to6server"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/stretchr/testify/require"
)

// Provider pairs a terraform provider name with an SDKv2 provider instance.
type Provider struct {
	Name     string
	Provider *schema.Provider
}

// Driver hosts TF providers in-process and runs the terraform CLI against them.
type Driver struct {
	cwd             string
	reattachConfigs map[string]*plugin.ReattachConfig
	// Env is passed through to the terraform subprocess (and therefore to any
	// provisioner shell commands) on top of os.Environ().
	Env map[string]string
}

func init() {
	for k, v := range map[string]string{
		"TF_LOG_PROVIDER":  "off",
		"TF_LOG_SDK":       "off",
		"TF_LOG_SDK_PROTO": "off",
	} {
		if err := os.Setenv(k, v); err != nil {
			panic(fmt.Sprintf("setting %s: %v", k, err))
		}
	}
}

// NewDriver creates a Driver for the given SDKv2 providers. If no providers are given,
// the driver runs terraform without any reattach configuration.
func NewDriver(t *testing.T, providers []Provider) *Driver {
	t.Helper()

	reattachConfigs := make(map[string]*plugin.ReattachConfig, len(providers))
	for _, p := range providers {
		v6server, err := tf5to6server.UpgradeServer(t.Context(),
			func() tfprotov5.ProviderServer { return p.Provider.GRPCProvider() })
		require.NoError(t, err)

		reattachConfigCh := make(chan *plugin.ReattachConfig)
		closeCh := make(chan struct{})

		serverOpts := []tf6server.ServeOpt{
			tf6server.WithGoPluginLogger(hclog.FromStandardLogger(log.New(io.Discard, "", 0), hclog.DefaultOptions)),
			tf6server.WithDebug(t.Context(), reattachConfigCh, closeCh),
			tf6server.WithoutLogStderrOverride(),
		}

		name := p.Name
		go func() {
			err := tf6server.Serve(name, func() tfprotov6.ProviderServer { return v6server }, serverOpts...)
			if err != nil {
				t.Logf("tf6server.Serve error: %v", err)
			}
		}()

		reattachConfigs[p.Name] = <-reattachConfigCh
	}

	return &Driver{
		cwd:             t.TempDir(),
		reattachConfigs: reattachConfigs,
	}
}

// Apply writes the input files, runs terraform init + apply, and returns all outputs.
// Config values are passed as -var flags to terraform apply.
//
// Apply may be called multiple times against the same Driver to drive a stack
// across stages; previous program files are removed before the new ones are
// written so a stage that drops a file doesn't leave the old one behind. State
// files (terraform.tfstate*, .terraform*) are kept across applies.
func (d *Driver) Apply(t *testing.T, input map[string]string, config map[string]string) map[string]string {
	t.Helper()
	outputs, err := d.TryApply(t, input, config)
	require.NoError(t, err)
	return outputs
}

// TryApply is like Apply but returns the error from `tofu apply` instead of
// failing the test. The outputs map is still parsed from terraform.tfstate when
// the file exists, so callers can inspect post-failure state.
func (d *Driver) TryApply(t *testing.T, input map[string]string, config map[string]string) (map[string]string, error) {
	t.Helper()

	require.NoError(t, removeProgramFiles(d.cwd))

	for path, content := range input {
		fullPath := filepath.Join(d.cwd, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
	}

	if _, err := d.execTf(t, "init", "-backend=false"); err != nil {
		return nil, err
	}

	applyArgs := append(make([]string, 0, 4+2*len(config)), "apply", "-auto-approve", "-refresh=false")
	for k, v := range config {
		applyArgs = append(applyArgs, "-var", k+"="+v)
	}
	if _, err := d.execTf(t, applyArgs...); err != nil {
		return d.tryParseOutputs(), err
	}
	return d.parseOutputs(t), nil
}

// Plan writes input files and runs `tofu plan`. Returns the error from the plan
// command — nil means the plan succeeded (deferred checks count as success).
func (d *Driver) Plan(t *testing.T, input map[string]string, config map[string]string) error {
	t.Helper()

	require.NoError(t, removeProgramFiles(d.cwd))

	for path, content := range input {
		fullPath := filepath.Join(d.cwd, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
	}

	if _, err := d.execTf(t, "init", "-backend=false"); err != nil {
		return err
	}
	planArgs := append(make([]string, 0, 3+2*len(config)), "plan", "-refresh=false")
	for k, v := range config {
		planArgs = append(planArgs, "-var", k+"="+v)
	}
	_, err := d.execTf(t, planArgs...)
	return err
}

// tryParseOutputs reads terraform.tfstate if it exists and returns its outputs.
// Returns an empty map when the file is missing, so callers can use it on
// failure paths without panicking.
func (d *Driver) tryParseOutputs() map[string]string {
	raw, err := os.ReadFile(filepath.Join(d.cwd, "terraform.tfstate"))
	if err != nil {
		return map[string]string{}
	}
	var state struct {
		Outputs map[string]struct {
			Value json.RawMessage `json:"value"`
		} `json:"outputs"`
	}
	if err := json.Unmarshal(raw, &state); err != nil {
		return map[string]string{}
	}
	result := make(map[string]string, len(state.Outputs))
	for k, v := range state.Outputs {
		var s string
		if err := json.Unmarshal(v.Value, &s); err == nil {
			result[k] = s
		} else {
			result[k] = string(v.Value)
		}
	}
	return result
}

// StateResources reads terraform.tfstate and returns the list of resource
// addresses present (e.g. "simple_resource.example"). Empty when the state
// file is missing.
func (d *Driver) StateResources(t *testing.T) []string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d.cwd, "terraform.tfstate"))
	if err != nil {
		return nil
	}
	var state struct {
		Resources []struct {
			Mode string `json:"mode"`
			Type string `json:"type"`
			Name string `json:"name"`
		} `json:"resources"`
	}
	require.NoError(t, json.Unmarshal(raw, &state))
	addrs := make([]string, 0, len(state.Resources))
	for _, r := range state.Resources {
		if r.Mode == "managed" {
			addrs = append(addrs, r.Type+"."+r.Name)
		}
	}
	return addrs
}

func (d *Driver) parseOutputs(t *testing.T) map[string]string {
	t.Helper()
	raw, err := os.ReadFile(filepath.Join(d.cwd, "terraform.tfstate"))
	require.NoError(t, err)

	var state struct {
		Outputs map[string]struct {
			Value json.RawMessage `json:"value"`
		} `json:"outputs"`
	}
	require.NoError(t, json.Unmarshal(raw, &state))

	result := make(map[string]string, len(state.Outputs))
	for k, v := range state.Outputs {
		var s string
		if err := json.Unmarshal(v.Value, &s); err == nil {
			result[k] = s
		} else {
			result[k] = string(v.Value)
		}
	}
	return result
}

func (d *Driver) formatReattachEnvVar() string {
	if len(d.reattachConfigs) == 0 {
		return ""
	}

	type reattachConfigAddr struct {
		Network string
		String  string
	}

	type reattachConfig struct {
		Protocol        string
		ProtocolVersion int
		Pid             int
		Test            bool
		Addr            reattachConfigAddr
	}

	configs := make(map[string]reattachConfig, len(d.reattachConfigs))
	for name, rc := range d.reattachConfigs {
		configs[name] = reattachConfig{
			Protocol:        string(rc.Protocol),
			ProtocolVersion: rc.ProtocolVersion,
			Pid:             rc.Pid,
			Test:            rc.Test,
			Addr: reattachConfigAddr{
				Network: rc.Addr.Network(),
				String:  rc.Addr.String(),
			},
		}
	}

	reattachBytes, err := json.Marshal(configs)
	if err != nil {
		panic(fmt.Sprintf("failed to build TF_REATTACH_PROVIDERS string: %v", err))
	}
	return "TF_REATTACH_PROVIDERS=" + string(reattachBytes)
}

func getTFCommand() string {
	if cmd := os.Getenv("TF_COMMAND_OVERRIDE"); cmd != "" {
		return cmd
	}
	return "tofu"
}

// removeProgramFiles deletes program files (everything except state and the
// .terraform plugin cache) from dir, so a subsequent Apply doesn't see leftover
// files from a previous stage.
func removeProgramFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		switch {
		case name == ".terraform", name == ".terraform.lock.hcl":
		case len(name) >= len("terraform.tfstate") && name[:len("terraform.tfstate")] == "terraform.tfstate":
		default:
			if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
				return err
			}
			continue
		}
	}
	return nil
}
