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
	"strings"
	"testing"

	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"
	"github.com/pulumi-labs/pulumi-hcl/vendored/getmodules"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// discoveryFor returns a discovery client that resolves apiHost to a tfe.v2 service on tfeHost, the
// way the real Pulumi Cloud backend's discovery document does.
func discoveryFor(t *testing.T, apiHost, tfeHost string) *disco.Disco {
	t.Helper()
	host, err := svchost.ForComparison(apiHost)
	require.NoError(t, err)
	d := disco.New()
	d.ForceHostServices(host, map[string]any{
		"tfe.v2": "https://" + tfeHost + "/api/v2",
	})
	return d
}

func TestDiscoverCloudRegistry(t *testing.T) {
	t.Parallel()

	// The host the engine passes as PULUMI_API differs from the registry host discovery resolves to,
	// and is dash-heavy, like a review stack. The registry host comes only from discovery.
	const (
		apiHost = "api-fnune-review.review-stacks.pulumi-dev.io"
		tfeHost = "tfe-fnune-review.review-stacks.pulumi-dev.io"
	)
	d := discoveryFor(t, apiHost, tfeHost)
	want, err := svchost.ForComparison(tfeHost)
	require.NoError(t, err)

	t.Run("logged in", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "https://"+apiHost, "the-token")
		require.NoError(t, err)
		require.NotNil(t, reg)
		assert.Equal(t, want, reg.host)
		assert.Equal(t, "the-token", reg.token)
	})

	t.Run("no token", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "https://"+apiHost, "")
		require.NoError(t, err)
		assert.Nil(t, reg)
	})

	t.Run("no backend address", func(t *testing.T) {
		t.Parallel()
		reg, err := discoverCloudRegistryWith(d, "", "the-token")
		require.NoError(t, err)
		assert.Nil(t, reg)
	})

	t.Run("backend without module registry", func(t *testing.T) {
		t.Parallel()
		bare := disco.New()
		host, err := svchost.ForComparison("diy.example.com")
		require.NoError(t, err)
		bare.ForceHostServices(host, map[string]any{})
		reg, err := discoverCloudRegistryWith(bare, "https://diy.example.com", "the-token")
		assert.Nil(t, reg)
		require.ErrorIs(t, err, errNoModuleRegistry)
	})
}

// TestCloudRegistryCredentialsLoggedOut confirms a logged-out session (empty backend address and
// token) injects no credentials and triggers no discovery, since the inputs short-circuit.
func TestCloudRegistryCredentialsLoggedOut(t *testing.T) {
	t.Parallel()
	creds := newCloudRegistryCredentials("", "")
	host, err := svchost.ForComparison("tfe.pulumi.com")
	require.NoError(t, err)
	hc, err := creds.ForHost(t.Context(), host)
	require.NoError(t, err)
	assert.Nil(t, hc)
}

// authHeaderFor applies creds for host to a throwaway request and returns the Authorization header
// they set, or "" when no credentials apply.
func authHeaderFor(t *testing.T, creds svcauth.CredentialsSource, host svchost.Hostname) string {
	t.Helper()
	hc, err := creds.ForHost(t.Context(), host)
	require.NoError(t, err)
	if hc == nil {
		return ""
	}
	req, err := http.NewRequest(http.MethodGet, "https://example.com", nil)
	require.NoError(t, err)
	hc.PrepareRequest(req)
	return req.Header.Get("Authorization")
}

func TestCredentialsForRegistry(t *testing.T) {
	t.Parallel()

	host, err := svchost.ForComparison("tfe.pulumi.com")
	require.NoError(t, err)
	reg := &cloudRegistry{host: host, token: "the-token"}
	creds := credentialsForRegistry(reg)
	require.NotNil(t, creds)

	t.Run("registry host gets the token", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "Bearer the-token", authHeaderFor(t, creds, host))
	})

	t.Run("third-party host gets nothing", func(t *testing.T) {
		t.Parallel()
		other, err := svchost.ForComparison("registry.terraform.io")
		require.NoError(t, err)
		assert.Equal(t, "", authHeaderFor(t, creds, other))
	})

	t.Run("no registry yields no credentials", func(t *testing.T) {
		t.Parallel()
		assert.Nil(t, credentialsForRegistry(nil))
	})
}

// TestRegistryGetScopesTokenToDiscoveredHost proves the end-to-end wiring: a networkResolver whose
// disco carries the cloud registry credentials sends the bearer token on its modules.v1 requests to
// the discovered host, and nothing to any other host.
func TestRegistryGetScopesTokenToDiscoveredHost(t *testing.T) {
	t.Parallel()

	regHost, err := svchost.ForComparison("tfe.pulumi.example.com")
	require.NoError(t, err)

	var gotAuth string
	mux := http.NewServeMux()
	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		gotAuth = r.Header.Get("Authorization")
		segs := strings.Split(strings.TrimPrefix(r.URL.Path, "/"), "/")
		switch {
		case len(segs) == 4 && segs[3] == "versions":
			_ = json.NewEncoder(w).Encode(map[string]any{
				"modules": []map[string]any{{"versions": versionsPayload([]string{"1.0.0"})}},
			})
		case len(segs) == 5 && segs[4] == "download":
			w.Header().Set("X-Terraform-Get", "https://example.com/m.tar.gz")
			w.WriteHeader(http.StatusNoContent)
		default:
			http.NotFound(w, r)
		}
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	d := disco.New(disco.WithCredentials(credentialsForRegistry(
		&cloudRegistry{host: regHost, token: "secret-token"})))
	n := &networkResolver{
		cacheDir: t.TempDir(),
		fetcher:  getmodules.NewPackageFetcher(t.Context(), nil),
		disco:    d,
	}

	// The discovered host receives the token.
	_, err = n.getRegistryDownloadURL(regHost, srv.URL, "acme", "thing", "aws", "")
	require.NoError(t, err)
	assert.Equal(t, "Bearer secret-token", gotAuth)

	// Any other host receives nothing.
	gotAuth = "sentinel"
	other, err := svchost.ForComparison("registry.terraform.io")
	require.NoError(t, err)
	_, err = n.getRegistryDownloadURL(other, srv.URL, "acme", "thing", "aws", "")
	require.NoError(t, err)
	assert.Equal(t, "", gotAuth)
}
