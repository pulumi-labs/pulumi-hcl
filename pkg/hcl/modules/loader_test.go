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
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/vendored/getmodules"
	"github.com/stretchr/testify/require"
)

// fakeRegistry serves the modules.v1 endpoints needed by getRegistryDownloadURL.
// versions is returned for /versions; downloads maps version → URL returned
// via the X-Terraform-Get header for /<v>/download (204 response).
type fakeRegistry struct {
	versions     []string
	downloads    map[string]string
	versionsHits int
	downloadHits map[string]int
}

func newFakeRegistry(t *testing.T, versions []string, downloads map[string]string) (*httptest.Server, *fakeRegistry) {
	t.Helper()
	reg := &fakeRegistry{
		versions:     versions,
		downloads:    downloads,
		downloadHits: map[string]int{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// modules.v1: /<ns>/<name>/<provider>/versions
		//             /<ns>/<name>/<provider>/<version>/download
		trimmed := strings.TrimPrefix(r.URL.Path, "/")
		segs := strings.Split(trimmed, "/")
		switch {
		case len(segs) == 4 && segs[3] == "versions":
			reg.versionsHits++
			payload := map[string]any{
				"modules": []map[string]any{
					{"versions": versionsPayload(reg.versions)},
				},
			}
			_ = json.NewEncoder(w).Encode(payload)
		case len(segs) == 5 && segs[4] == "download":
			v := segs[3]
			reg.downloadHits[v]++
			dl, ok := reg.downloads[v]
			if !ok {
				http.Error(w, "no such version", http.StatusNotFound)
				return
			}
			w.Header().Set("X-Terraform-Get", dl)
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	return srv, reg
}

func versionsPayload(vs []string) []map[string]string {
	out := make([]map[string]string, len(vs))
	for i, v := range vs {
		out[i] = map[string]string{"version": v}
	}
	return out
}

func newTestLoader(t *testing.T, registryBaseURL string) *Loader {
	t.Helper()
	return &Loader{
		parser:          parser.NewParser(),
		cache:           map[string]*LoadedModule{},
		cacheDir:        t.TempDir(),
		fetcher:         getmodules.NewPackageFetcher(t.Context(), nil),
		registryBaseURL: registryBaseURL,
	}
}

func TestGetRegistryDownloadURL_NoConstraintPicksLatest(t *testing.T) {
	t.Parallel()
	// Registry orders Versions oldest-first. We must not pick versions[0].
	srv, reg := newFakeRegistry(t,
		[]string{"1.0.0", "3.2.1", "4.0.0", "4.2.0", "2.9.9"},
		map[string]string{"4.2.0": "https://example.com/v420.tar.gz"})
	l := newTestLoader(t, srv.URL)

	got, err := l.getRegistryDownloadURL("acme", "thing", "aws", "")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v420.tar.gz", got)
	require.Equal(t, 1, reg.versionsHits)
	require.Equal(t, 1, reg.downloadHits["4.2.0"])
}

func TestGetRegistryDownloadURL_ConstraintPicksHighestMatching(t *testing.T) {
	t.Parallel()
	srv, reg := newFakeRegistry(t,
		[]string{"3.0.0", "3.5.0", "4.0.0", "4.0.1", "4.1.0", "5.0.0"},
		map[string]string{"4.1.0": "https://example.com/v410.tar.gz"})
	l := newTestLoader(t, srv.URL)

	got, err := l.getRegistryDownloadURL("acme", "thing", "aws", "~> 4.0")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v410.tar.gz", got)
	require.Equal(t, 1, reg.downloadHits["4.1.0"])
}

func TestGetRegistryDownloadURL_ExactVersionPin(t *testing.T) {
	t.Parallel()
	srv, _ := newFakeRegistry(t,
		[]string{"1.0.0", "2.0.0", "3.0.0"},
		map[string]string{"2.0.0": "https://example.com/v200.tar.gz"})
	l := newTestLoader(t, srv.URL)

	got, err := l.getRegistryDownloadURL("acme", "thing", "aws", "2.0.0")
	require.NoError(t, err)
	require.Equal(t, "https://example.com/v200.tar.gz", got)
}

func TestGetRegistryDownloadURL_NoMatchingVersionErrors(t *testing.T) {
	t.Parallel()
	srv, _ := newFakeRegistry(t,
		[]string{"1.0.0", "2.0.0"},
		map[string]string{})
	l := newTestLoader(t, srv.URL)

	_, err := l.getRegistryDownloadURL("acme", "thing", "aws", "~> 99.0")
	require.Error(t, err)
	require.Contains(t, err.Error(), "no published version")
	require.Contains(t, err.Error(), "~> 99.0")
}

// TestGetRegistryDownloadURL_OpenTofuStyle_JSONBody verifies the OpenTofu
// registry response shape: 200 + JSON `{"location":"..."}` body, rather than
// the 204 + X-Terraform-Get header used by the Terraform registry.
func TestGetRegistryDownloadURL_OpenTofuStyle_JSONBody(t *testing.T) {
	t.Parallel()

	const wantURL = "git::https://example.com/repo?ref=abc123"
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		trimmed := strings.TrimPrefix(r.URL.Path, "/")
		segs := strings.Split(trimmed, "/")
		switch {
		case len(segs) == 4 && segs[3] == "versions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"modules": []map[string]any{
					{"versions": versionsPayload([]string{"1.0.0"})},
				},
			})
		case len(segs) == 5 && segs[4] == "download":
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"location": "` + wantURL + `"}`))
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	l := newTestLoader(t, srv.URL)
	got, err := l.getRegistryDownloadURL("acme", "thing", "aws", "")
	require.NoError(t, err)
	require.Equal(t, wantURL, got)
}

func TestGetRegistryDownloadURL_InvalidConstraintErrors(t *testing.T) {
	t.Parallel()
	srv, _ := newFakeRegistry(t,
		[]string{"1.0.0"},
		map[string]string{"1.0.0": "https://example.com/x.tar.gz"})
	l := newTestLoader(t, srv.URL)

	_, err := l.getRegistryDownloadURL("acme", "thing", "aws", "not-a-constraint")
	require.Error(t, err)
	require.Contains(t, err.Error(), "parsing version constraint")
}

func TestLoadModule_VersionConstraintPlumbedThroughToRegistry(t *testing.T) {
	t.Parallel()

	// Serve a module tarball that the registry will redirect us to. We pin
	// 4.0.1 as the only version-with-a-download so a successful load *proves*
	// the `~> 4.0` constraint selected 4.0.1 rather than 5.1.0 or 3.x.
	modTar := buildModuleTarGz(t, map[string]string{
		"main.tf": `output "ok" { value = "v4-output" }`,
	})
	tarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/gzip")
		_, _ = w.Write(modTar)
	}))
	t.Cleanup(tarSrv.Close)
	// go-getter sniffs the archive type from the URL; force tar.gz so the
	// HTTP getter knows which decompressor to use.
	tarURL := tarSrv.URL + "/m.tar.gz?archive=tar.gz"

	regSrv, reg := newFakeRegistry(t,
		[]string{"3.0.0", "3.9.9", "4.0.0", "4.0.1", "5.1.0"},
		map[string]string{"4.0.1": tarURL})
	l := newTestLoader(t, regSrv.URL)

	loaded, err := l.LoadModule("acme/thing/aws", "~> 4.0", t.TempDir())
	require.NoError(t, err)
	require.NotNil(t, loaded.Config)
	require.Contains(t, loaded.Config.Outputs, "ok",
		"the module body fetched should be the 4.0.1 fixture")
	require.Equal(t, 1, reg.downloadHits["4.0.1"],
		"constraint ~> 4.0 should resolve to 4.0.1 (highest 4.x), not 5.1.0 or 3.x")
}

func TestLoadModule_VersionQueryStringStillSupported(t *testing.T) {
	t.Parallel()
	modTar := buildModuleTarGz(t, map[string]string{
		"main.tf": `output "ok" { value = "v3-output" }`,
	})
	tarSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(modTar)
	}))
	t.Cleanup(tarSrv.Close)
	tarURL := tarSrv.URL + "/m.tar.gz?archive=tar.gz"

	regSrv, reg := newFakeRegistry(t,
		[]string{"3.0.0", "4.0.0"},
		map[string]string{"3.0.0": tarURL})
	l := newTestLoader(t, regSrv.URL)

	_, err := l.LoadModule("acme/thing/aws?version=3.0.0", "", t.TempDir())
	require.NoError(t, err)
	require.Equal(t, 1, reg.downloadHits["3.0.0"])
}

// buildModuleTarGz packs `files` (name → content) into a gzipped tar archive
// using the system `tar`. The returned bytes are the raw .tar.gz body that
// PackageFetcher / go-getter can consume.
func buildModuleTarGz(t *testing.T, files map[string]string) []byte {
	t.Helper()
	src := t.TempDir()
	for name, content := range files {
		full := filepath.Join(src, name)
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte(content), 0o600))
	}
	out := filepath.Join(t.TempDir(), "out.tar.gz")
	cmd := exec.Command("tar", "-czf", out, "-C", src, ".")
	// macOS BSD tar otherwise writes AppleDouble `._foo` companions which
	// our parser rejects as invalid HCL when the tarball is later extracted.
	cmd.Env = append(os.Environ(), "COPYFILE_DISABLE=1")
	combined, err := cmd.CombinedOutput()
	require.NoErrorf(t, err, "tar: %s", combined)
	data, err := os.ReadFile(out)
	require.NoError(t, err)
	return data
}
