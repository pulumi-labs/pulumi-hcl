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

package run

import (
	"context"
	"strings"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	pulumischema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// resourceBodyMapping returns the bridge BodyMapping for a TF resource type
// (e.g. "aws_s3_bucket"), or nil when the mapper is unavailable, the
// provider isn't known, or the type isn't in its schema.
func (e *Engine) resourceBodyMapping(ctx context.Context, tfResourceType string) *bridge.BodyMapping {
	info := e.providerInfoForType(ctx, tfResourceType)
	if info == nil {
		return nil
	}
	return bridge.ResourceBodyMapping(info, tfResourceType)
}

// dataSourceBodyMapping mirrors resourceBodyMapping for a TF data source.
func (e *Engine) dataSourceBodyMapping(ctx context.Context, tfType string) *bridge.BodyMapping {
	info := e.providerInfoForType(ctx, tfType)
	if info == nil {
		return nil
	}
	return bridge.DataSourceBodyMapping(info, tfType)
}

// providerConfigBodyMapping returns the BodyMapping for a provider block by
// TF provider local name (e.g. "aws").
func (e *Engine) providerConfigBodyMapping(ctx context.Context, providerName string) *bridge.BodyMapping {
	info := e.providerInfoFor(ctx, providerName)
	if info == nil {
		return nil
	}
	return bridge.ProviderConfigBodyMapping(info)
}

// providerInfoForType resolves the TF provider that owns a resource or data
// source token (split on first underscore) and fetches its bridge info. For
// single-segment tokens — providers like hashicorp/external or hashicorp/http
// whose type name equals the provider name — the whole token is the provider
// name, so the lookup falls through to that.
func (e *Engine) providerInfoForType(ctx context.Context, tfType string) *tfbridge.ProviderInfo {
	if idx := strings.IndexByte(tfType, '_'); idx > 0 {
		return e.providerInfoFor(ctx, tfType[:idx])
	}
	if tfType == "" {
		return nil
	}
	return e.providerInfoFor(ctx, tfType)
}

// resolveResource resolves a TF resource type to a Pulumi schema.Resource.
// It consults the bridge mapping first (so e.g. `aws_canonical_user_id` maps
// directly to `aws:s3/getCanonicalUserId:getCanonicalUserId`, no matter how
// Pulumi has organised the type into modules) and falls back to the
// convention-based resolver in pkg/hcl/packages when no mapping is available.
func (e *Engine) resolveResource(ctx context.Context, tfType string) (*pulumischema.Resource, error) {
	if tfType == terraformDataType {
		return terraformDataSchema(), nil
	}
	if info := e.providerInfoForType(ctx, tfType); info != nil {
		if r, ok := info.Resources[tfType]; ok && r != nil && string(r.Tok) != "" {
			res, err := e.loadResourceByToken(ctx, info.Name, string(r.Tok))
			if err == nil {
				return res, nil
			}
			logging.V(5).Infof("bridge resource token %q (from %q) not loadable: %v", r.Tok, tfType, err)
		}
	}
	return packages.ResolveResource(ctx, e.pkgLoader, e.knownProviders(), tfType)
}

// resolveFunction mirrors resolveResource for TF data sources and functions.
func (e *Engine) resolveFunction(ctx context.Context, tfType string) (*pulumischema.Function, error) {
	if info := e.providerInfoForType(ctx, tfType); info != nil {
		if d, ok := info.DataSources[tfType]; ok && d != nil && string(d.Tok) != "" {
			fn, err := e.loadFunctionByToken(ctx, info.Name, string(d.Tok))
			if err == nil {
				return fn, nil
			}
			logging.V(5).Infof("bridge data source token %q (from %q) not loadable: %v", d.Tok, tfType, err)
		}
	}
	return packages.ResolveFunction(ctx, e.pkgLoader, e.knownProviders(), tfType)
}

// loadResourceByToken loads the Pulumi schema for an exact Pulumi token.
func (e *Engine) loadResourceByToken(ctx context.Context, pkgName, tok string) (*pulumischema.Resource, error) {
	pkg, err := e.pkgLoader.LoadPackageReferenceV2(ctx, &pulumischema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, err
	}
	r, _, err := pkg.Resources().Get(tok)
	return r, err
}

// loadFunctionByToken loads the Pulumi schema for an exact Pulumi function token.
func (e *Engine) loadFunctionByToken(ctx context.Context, pkgName, tok string) (*pulumischema.Function, error) {
	pkg, err := e.pkgLoader.LoadPackageReferenceV2(ctx, &pulumischema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, err
	}
	fn, _, err := pkg.Functions().Get(tok)
	return fn, err
}

// providerInfoFor fetches the bridge ProviderInfo for a TF provider local
// name, threading the SDK-on-disk descriptor (when present) through to the
// mapper so dynamically bridged providers route via the correct
// parameterization. Returns nil on miss (and logs at V(5) so the fallback
// path is still visible during debugging).
func (e *Engine) providerInfoFor(ctx context.Context, tfProvider string) *tfbridge.ProviderInfo {
	if e.providerInfoSource == nil {
		return nil
	}
	var desc *workspace.PackageDescriptor
	if d, ok := e.packages[tfProvider]; ok {
		desc = &d
	}
	info, err := e.providerInfoSource.GetProviderInfo(ctx, tfProvider, desc)
	if err != nil {
		logging.V(5).Infof("bridge mapping unavailable for %q: %v", tfProvider, err)
		return nil
	}
	return info
}
