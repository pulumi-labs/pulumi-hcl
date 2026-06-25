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
	"context"
	"errors"
	"fmt"
	"net/url"
	"sync"

	"github.com/opentofu/svchost"
	"github.com/opentofu/svchost/disco"
	"github.com/opentofu/svchost/svcauth"
)

// errNoModuleRegistry means the backend's discovery document advertises no module registry, so there
// is nothing to authenticate against.
var errNoModuleRegistry = errors.New("backend does not host a Pulumi Cloud module registry")

// cloudRegistry is the Pulumi Cloud module registry the loader was launched against, plus the access
// token to authenticate to it. Both come from discovery against the engine-provided backend address:
// there is no other source of the registry host.
type cloudRegistry struct {
	host  svchost.Hostname
	token string
}

func discoverCloudRegistry(apiAddress, token string) (*cloudRegistry, error) {
	return discoverCloudRegistryWith(disco.New(), apiAddress, token)
}

func discoverCloudRegistryWith(d *disco.Disco, apiAddress, token string) (*cloudRegistry, error) {
	if apiAddress == "" || token == "" {
		return nil, nil
	}
	apiURL, err := url.Parse(apiAddress)
	if err != nil {
		return nil, fmt.Errorf("parsing backend address %q: %w", apiAddress, err)
	}
	if apiURL.Host == "" {
		return nil, fmt.Errorf("backend address %q has no host", apiAddress)
	}
	apiHost, err := svchost.ForComparison(apiURL.Host)
	if err != nil {
		return nil, fmt.Errorf("normalizing backend host %q: %w", apiURL.Host, err)
	}
	serviceURL, err := d.DiscoverServiceURL(context.Background(), apiHost, "tfe.v2")
	if err != nil {
		var notProvided *disco.ErrServiceNotProvided
		if errors.As(err, &notProvided) {
			return nil, errNoModuleRegistry
		}
		return nil, fmt.Errorf("discovering module registry for %q: %w", apiHost, err)
	}
	if serviceURL == nil {
		return nil, errNoModuleRegistry
	}
	registryHost, err := svchost.ForComparison(serviceURL.Host)
	if err != nil {
		return nil, fmt.Errorf("normalizing registry host %q: %w", serviceURL.Host, err)
	}
	return &cloudRegistry{host: registryHost, token: token}, nil
}

// cloudRegistryCredentials authenticates module registry requests to the Pulumi Cloud registry
// without a separate `terraform login`. It is built from the engine-provided backend address and
// access token at the loader's construction and threaded into its disco client. Discovery is
// deferred to the first credential lookup and memoized, so a program with no registry modules pays
// no discovery cost, and a logged-out session (empty address or token) injects nothing.
type cloudRegistryCredentials struct {
	apiAddress string
	token      string

	once sync.Once
	src  svcauth.CredentialsSource // scoped to the discovered host; nil when there is none
	err  error
}

func newCloudRegistryCredentials(apiAddress, token string) *cloudRegistryCredentials {
	return &cloudRegistryCredentials{apiAddress: apiAddress, token: token}
}

// ForHost implements [svcauth.CredentialsSource]. A discovery failure here surfaces to the user as
// an error from the subsequent registry request (a 401 for a private module), so the host is left
// unauthenticated rather than failing the lookup.
func (c *cloudRegistryCredentials) ForHost(ctx context.Context, host svchost.Hostname) (svcauth.HostCredentials, error) {
	c.once.Do(func() {
		reg, err := discoverCloudRegistry(c.apiAddress, c.token)
		c.err, c.src = err, credentialsForRegistry(reg)
	})
	if c.err != nil || c.src == nil {
		return nil, nil
	}
	return c.src.ForHost(ctx, host)
}

// credentialsForRegistry scopes the access token to reg's host: it returns the token for that host
// and nothing for any other. It returns nil when reg is nil.
func credentialsForRegistry(reg *cloudRegistry) svcauth.CredentialsSource {
	if reg == nil {
		return nil
	}
	return svcauth.StaticCredentialsSource(map[svchost.Hostname]svcauth.HostCredentials{
		reg.host: svcauth.HostCredentialsToken(reg.token),
	})
}
