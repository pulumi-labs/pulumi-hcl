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

package pulexec

import (
	"context"

	p "github.com/pulumi/pulumi-go-provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// NativeProvider serves a native pulumi-go-provider.
//
// Unset config/CRUD methods are filled with succeeding echo stubs.
func NativeProvider(name, version string, prov p.Provider) Provider {
	echoCheck := func(_ context.Context, req p.CheckRequest) (p.CheckResponse, error) {
		return p.CheckResponse{Inputs: req.Inputs}, nil
	}
	noDiff := func(context.Context, p.DiffRequest) (p.DiffResponse, error) {
		return p.DiffResponse{}, nil
	}
	if prov.CheckConfig == nil {
		prov.CheckConfig = echoCheck
	}
	if prov.DiffConfig == nil {
		prov.DiffConfig = noDiff
	}
	if prov.Configure == nil {
		prov.Configure = func(context.Context, p.ConfigureRequest) error { return nil }
	}
	if prov.Check == nil {
		prov.Check = echoCheck
	}
	if prov.Diff == nil {
		prov.Diff = noDiff
	}
	if prov.Create == nil {
		prov.Create = func(_ context.Context, req p.CreateRequest) (p.CreateResponse, error) {
			return p.CreateResponse{ID: "id-0", Properties: req.Properties}, nil
		}
	}
	factory := p.RawServer(name, version, prov)
	return Provider{Name: name, Start: func(context.Context) (pulumirpc.ResourceProviderServer, error) {
		return factory(nil)
	}}
}
