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

// Package resolve turns a module's provider requirements into concrete
// [workspace.PackageDescriptor]s using the engine's package-resolver service.
package resolve

import (
	"context"
	"fmt"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
)

// Request names one provider to resolve. Alias is the provider's local name in
// the module (e.g. "aws"); it keys the resulting descriptor so the engine and
// the bridge mapper can look the descriptor up the same way they do for on-disk
// SDKs. Spec is the package specification handed to the resolver.
type Request struct {
	Alias string
	Spec  *pulumirpc.PackageSpec
}

// Packages resolves each request through the resolver and returns the resolved
// descriptors keyed by alias.
func Packages(
	ctx context.Context, resolver pulumirpc.PackageResolverClient, reqs []Request,
) (map[string]workspace.PackageDescriptor, error) {
	out := make(map[string]workspace.PackageDescriptor, len(reqs))
	for _, r := range reqs {
		dep, err := resolver.ResolvePackage(ctx, r.Spec)
		if err != nil {
			return nil, fmt.Errorf("resolving provider %q: %w", r.Alias, err)
		}
		desc, err := dependencyToDescriptor(dep)
		if err != nil {
			return nil, fmt.Errorf("provider %q: %w", r.Alias, err)
		}
		out[r.Alias] = desc
	}
	return out, nil
}

func dependencyToDescriptor(dep *pulumirpc.PackageDependency) (workspace.PackageDescriptor, error) {
	contract.Requiref(dep.Kind != "", "dep.kind", "cannot be the empty string")
	var version *semver.Version
	if dep.Version != "" {
		v, err := semver.ParseTolerant(dep.Version)
		if err != nil {
			return workspace.PackageDescriptor{}, fmt.Errorf("parsing version %q: %w", dep.Version, err)
		}
		version = &v
	}

	kind := apitype.PluginKind(dep.Kind)

	param, err := parameterization(dep.Parameterization)
	if err != nil {
		return workspace.PackageDescriptor{}, err
	}
	ext, err := parameterization(dep.Extension)
	if err != nil {
		return workspace.PackageDescriptor{}, err
	}

	return workspace.PackageDescriptor{
		PluginDescriptor: workspace.PluginDescriptor{
			Name:              dep.Name,
			Kind:              kind,
			Version:           version,
			PluginDownloadURL: dep.Server,
			Checksums:         dep.Checksums,
		},
		Parameterization:          param,
		ExtensionParameterization: ext,
	}, nil
}

func parameterization(p *pulumirpc.PackageParameterization) (*workspace.Parameterization, error) {
	if p == nil {
		return nil, nil
	}
	v, err := semver.ParseTolerant(p.Version)
	if err != nil {
		return nil, fmt.Errorf("parsing parameterization version %q: %w", p.Version, err)
	}
	return &workspace.Parameterization{
		Name:    p.Name,
		Version: v,
		Value:   p.Value,
	}, nil
}
