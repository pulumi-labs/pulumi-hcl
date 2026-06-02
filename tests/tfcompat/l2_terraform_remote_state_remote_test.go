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

package tfcompat_test

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/stretchr/testify/require"
)

const (
	// tfeHost is Terraform Cloud. The test targets TFC, not a self-hosted
	// Terraform Enterprise, so the hostname is fixed.
	tfeHost = "app.terraform.io"
	// tfeTokenEnv is the TF_TOKEN_<host> credential variable the cloud backend
	// reads to authenticate to tfeHost.
	tfeTokenEnv = "TF_TOKEN_app_terraform_io"
)

// TestL2TerraformRemoteStateRemote reads a Terraform Cloud workspace's outputs
// through terraform_remote_state's `remote` backend. It is a live test: it
// creates and seeds a throwaway TFC workspace, drives both `tofu apply` (builtin
// remote backend) and `pulumi up` (pulumi-terraform getRemoteReference) against
// it, and asserts identical outputs. It is skipped unless TFE_ORGANIZATION and
// TFE_TOKEN are set; the workspace name, organization, hostname, and token are
// injected as config vars so nothing sensitive is committed.
func TestL2TerraformRemoteStateRemote(t *testing.T) {
	t.Parallel()

	org := getEnv(t, "TFE_ORGANIZATION")
	token := getEnv(t, "TFE_TOKEN")

	workspace := "pulumi-hcl-tfcompat-" + randomSuffix(t)
	createRemoteWorkspace(t, org, token, workspace)
	t.Cleanup(func() { deleteRemoteWorkspace(t, org, token, workspace) })
	seedRemoteWorkspace(t, org, token, workspace)

	tfcompat.RunCase(t, "l2_terraform_remote_state_remote", tfcompat.Case{
		Config: map[string]string{
			"org":       org,
			"hostname":  tfeHost,
			"token":     token,
			"workspace": workspace,
		},
	})
}

func getEnv(t *testing.T, env string) string {
	value := os.Getenv(env)
	if value == "" {
		stopf := t.Fatalf
		if os.Getenv("CI") == "" {
			stopf = t.Skipf
		}
		stopf("Could not find %s", env)
	}
	return value
}

func randomSuffix(t *testing.T) string {
	t.Helper()
	b := make([]byte, 6)
	_, err := rand.Read(b)
	require.NoError(t, err)
	return hex.EncodeToString(b)
}

// tfeAPI issues a Terraform Cloud API request and returns the status code and body.
func tfeAPI(t *testing.T, method, path, token, body string) (int, []byte) {
	t.Helper()
	var r io.Reader
	if body != "" {
		r = strings.NewReader(body)
	}
	req, err := http.NewRequest(method, "https://"+tfeHost+path, r)
	require.NoError(t, err)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Type", "application/vnd.api+json")
	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer contract.IgnoreClose(resp.Body)
	data, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, data
}

// createRemoteWorkspace creates a workspace with local execution mode, so the
// seeding `tofu apply` runs locally and pushes its state to TFC.
func createRemoteWorkspace(t *testing.T, org, token, name string) {
	t.Helper()
	body := fmt.Sprintf(`{"data":{"type":"workspaces","attributes":{"name":%q,"execution-mode":"local"}}}`, name)
	code, data := tfeAPI(t, http.MethodPost, "/api/v2/organizations/"+org+"/workspaces", token, body)
	require.Equalf(t, http.StatusCreated, code, "create workspace %q: %s", name, data)
}

func deleteRemoteWorkspace(t *testing.T, org, token, name string) {
	t.Helper()
	code, data := tfeAPI(t, http.MethodDelete, "/api/v2/organizations/"+org+"/workspaces/"+name, token, "")
	if code != http.StatusNoContent && code != http.StatusNotFound {
		t.Logf("cleanup: deleting workspace %q returned %d: %s", name, code, data)
	}
}

// seedRemoteWorkspace applies a tiny config with known outputs to the workspace
// via the cloud backend, so terraform_remote_state has a state version to read.
func seedRemoteWorkspace(t *testing.T, org, token, name string) {
	t.Helper()
	dir := t.TempDir()
	program := fmt.Sprintf(`terraform {
  cloud {
    hostname     = %q
    organization = %q
    workspaces { name = %q }
  }
}
output "greeting" { value = "hello" }
output "number"   { value = 42 }
`, tfeHost, org, name)
	require.NoError(t, os.WriteFile(filepath.Join(dir, "main.tf"), []byte(program), 0o600))

	tf := os.Getenv("TF_COMMAND_OVERRIDE")
	if tf == "" {
		tf = "tofu"
	}
	env := append(os.Environ(), tfeTokenEnv+"="+token)
	for _, args := range [][]string{{"init", "-input=false"}, {"apply", "-auto-approve", "-input=false"}} {
		cmd := exec.Command(tf, args...) //nolint:gosec // tf and args are test-controlled
		cmd.Dir = dir
		cmd.Env = env
		out, err := cmd.CombinedOutput()
		require.NoErrorf(t, err, "seeding %q with %s %v: %s", name, tf, args, out)
	}
}
