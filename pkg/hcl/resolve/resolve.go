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
	"crypto/des"
	"fmt"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/trace"
	"golang.org/x/sync/errgroup"

	"github.com/pulumi-labs/pulumi-hcl/pkg/potel"
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
	type response struct {
		alias      string
		descriptor workspace.PackageDescriptor
	}
	out := make([]response, len(reqs))
	var wg errgroup.Group
	for i, r := range reqs {
		wg.Go(func() error {
			dep, err := resolveOne(ctx, resolver, r)
			if err != nil {
				return fmt.Errorf("resolving provider %q: %w", r.Alias, err)
			}
			desc, err := dependencyToDescriptor(dep)
			if err != nil {
				return fmt.Errorf("provider %q: %w", r.Alias, err)
			}
			out[i] = response{r.Alias, desc}
			return nil
		})
	}

	if err := wg.Wait(); err != nil {
		return nil, err
	}

	outMap := make(map[string]workspace.PackageDescriptor, len(reqs))
	for _, v := range out {
		outMap[v.alias] = v.descriptor
	}

	return outMap, nil
}

// resolveOne resolves a single request, recording a span so the per-provider
// cost (the engine installing and parameterizing the provider plugin) is
// visible in traces.
func resolveOne(
	ctx context.Context, resolver pulumirpc.PackageResolverClient, r Request,
) (*pulumirpc.PackageDependency, error) {
	ctx, span := potel.Start(ctx, "resolve.ResolvePackage",
		trace.WithAttributes(
			attribute.String("alias", r.Alias),
			attribute.String("version", r.Spec.GetVersion()),
			attribute.String("source", r.Spec.GetSource()),
		))
	defer span.End()
	return resolver.ResolvePackage(ctx, r.Spec)
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
