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
	"archive/tar"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/stretchr/testify/require"
)

// TestL2ModuleRegistryAuth resolves a registry module from a private modules.v1
// registry that requires a bearer token, authenticated only via the standard
// Terraform mechanism: a TF_TOKEN_<host> environment variable plus a CLI-config
// `host` block redirecting the (non-resolvable) registry host to a local server.
//
// OpenTofu honors both, so `tofu init` authenticates and resolves the module.
// pulumi-hcl must reach parity: respect TF_TOKEN_<host> and the CLI-config host
// service override so the same private module resolves with no `terraform login`.
//
// The test is intentionally provider-free — the served module declares only an
// output — so it isolates module resolution and registry authentication.
func TestL2ModuleRegistryAuth(t *testing.T) {
	const (
		registryHost = "registry.tfcompat.test"
		token        = "s3cr3t-registry-token"
	)

	srv, authedHits := startAuthedModuleRegistry(t, token)

	cliConfig := writeTerraformCLIConfig(t, registryHost, srv.URL+"/v1/modules/")

	t.Setenv("TF_CLI_CONFIG_FILE", cliConfig)
	t.Setenv("TF_TOKEN_"+strings.ReplaceAll(registryHost, ".", "_"), token)

	tfcompat.RunCase(t, "l2_module_registry_auth", tfcompat.Case{})

	// The registry only answers authenticated callers, so any resolution at all
	// proves the bearer token was sent — the behavior under test, not bypassed.
	require.Positive(t, authedHits.Load(),
		"the registry should have served at least one authenticated modules.v1 request")
}

// startAuthedModuleRegistry serves the modules.v1 protocol for
// acme/widget/aws@1.0.0, requiring `Authorization: Bearer <token>` on the
// versions and download endpoints. The module archive itself is served
// unauthenticated, the way a registry hands back a pre-signed download URL.
func startAuthedModuleRegistry(t *testing.T, token string) (*httptest.Server, *atomic.Int64) {
	t.Helper()

	moduleArchive := buildModuleArchive(t, map[string]string{
		"main.tf": `output "greeting" { value = "from-registry-module" }` + "\n",
	})

	var authedHits atomic.Int64
	mux := http.NewServeMux()

	// The module archive: unauthenticated, like a pre-signed object URL. The
	// ?archive=tar.gz hint tells go-getter which decompressor to use.
	mux.HandleFunc("/download/module.tar.gz", func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(moduleArchive)
	})

	// modules.v1: /v1/modules/<ns>/<name>/<system>/(versions|<version>/download).
	// Both require the bearer token; an unauthenticated request gets a 401, the
	// way a private registry refuses an anonymous caller.
	mux.HandleFunc("/v1/modules/", func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer "+token {
			http.Error(w, "missing or wrong bearer token", http.StatusUnauthorized)
			return
		}
		authedHits.Add(1)
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/v1/modules/"), "/")
		switch {
		case len(segs) == 4 && segs[3] == "versions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"modules": []map[string]any{
					{"versions": []map[string]string{{"version": "1.0.0"}}},
				},
			})
		case len(segs) == 5 && segs[4] == "download":
			w.Header().Set("X-Terraform-Get", "http://"+r.Host+"/download/module.tar.gz?archive=tar.gz")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, &authedHits
}

// writeTerraformCLIConfig writes a Terraform/OpenTofu CLI config file with a
// `host` block that overrides service discovery for host, pointing its
// modules.v1 service at modulesV1URL (a local server). It returns the path, to
// be exported via TF_CLI_CONFIG_FILE.
func writeTerraformCLIConfig(t *testing.T, host, modulesV1URL string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "tfcompat.tfrc")
	body := fmt.Sprintf(`host %q {
  services = {
    "modules.v1" = %q
  }
}
`, host, modulesV1URL)
	require.NoError(t, os.WriteFile(path, []byte(body), 0o600))
	return path
}

// buildModuleArchive returns a gzip-compressed tarball of files, built in
// memory so it carries none of the macOS AppleDouble (`._*`) sidecars that
// shelling out to `tar` would add.
func buildModuleArchive(t *testing.T, files map[string]string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, content := range files {
		require.NoError(t, tw.WriteHeader(&tar.Header{
			Name: name,
			Mode: 0o600,
			Size: int64(len(content)),
		}))
		_, err := tw.Write([]byte(content))
		require.NoError(t, err)
	}
	require.NoError(t, tw.Close())
	require.NoError(t, gz.Close())
	return buf.Bytes()
}
