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

// Package packages handles Pulumi package schema loading and type mapping.
package packages

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"strings"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

var ErrNotFound = errors.New("not found")

type InvalidToken struct {
	token, reason string
}

func (err InvalidToken) Error() string {
	var b strings.Builder
	b.WriteString("invalid token")
	if err.token != "" {
		b.WriteRune(' ')
		b.WriteString(strconv.Quote(err.token))
	}
	if err.reason != "" {
		b.WriteRune(' ')
		b.WriteString(err.reason)
	}
	return b.String()
}

func PulumiTokenToHCL(token string) (string, hcl.Diagnostics) {
	if token == "pulumi:pulumi:StackReference" {
		return "pulumi_stackreference", nil
	}
	pkg, mod, name, diag := pcl.DecomposeToken(token, hcl.Range{})
	if diag.HasErrors() {
		return "", diag
	}
	hclToken := pkg
	if mod != "index" && mod != "" {
		hclToken += "_" + strings.ToLower(strings.ReplaceAll(mod, "/", "_"))
	}
	return hclToken + "_" + strings.ToLower(strings.ReplaceAll(name, "/", "_")), nil
}

// packageFromToken determines the package name from an HCL token and the list of
// known providers from required_providers.
//
// If exactly one known provider matches as a prefix of the token, that provider
// name is returned. If multiple known providers match, an error is returned to
// surface the ambiguity. If no known provider matches, the first underscore-
// delimited segment is used as the package name.
func packageFromToken(knownProviders []string, token string) (string, error) {
	var matches []string
	for _, p := range knownProviders {
		if strings.HasPrefix(token, p+"_") {
			matches = append(matches, p)
		}
	}
	switch len(matches) {
	case 0:
		return token[:strings.Index(token, "_")], nil
	case 1:
		return matches[0], nil
	default:
		return "", fmt.Errorf("ambiguous token %q: matches multiple providers %v", token, matches)
	}
}

func ResolveResource(ctx context.Context, loader schema.ReferenceLoader, knownProviders []string, token string) (*schema.Resource, error) {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return nil, InvalidToken{token: token, reason: "Pulumi HCL tokens must have at least 2 parts"}
	}

	if provider, ok := strings.CutPrefix(token, "pulumi_providers_"); ok {
		pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: provider})
		if err != nil {
			return nil, err
		}
		return pkg.Provider()
	}

	// Prevent users from needing to write pulumi_pulumi_stackreference
	if token == "pulumi_stackreference" {
		pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: "pulumi"})
		if err != nil {
			return nil, err
		}
		r, ok, err := pkg.Resources().Get("pulumi:pulumi:StackReference")
		contract.Assertf(ok, "stack references are there")
		return r, err
	}

	pkgName, err := packageFromToken(knownProviders, token)
	if err != nil {
		return nil, err
	}

	pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, ErrNotFound
	}

	key := strings.ReplaceAll(token[len(pkgName)+1:], "_", "")
	for iter := pkg.Resources().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		rKey := strings.NewReplacer("/", "", "_", "").Replace(strings.ToLower(mod + name))
		if rKey == key {
			return iter.Resource()
		}
	}
	return nil, ErrNotFound
}

func resolvePackage(ctx context.Context, loader schema.ReferenceLoader, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	if descriptor.Name == "pulumi" {
		return schema.DefaultPulumiPackage.Reference(), nil
	}

	pkg, err := loader.LoadPackageReferenceV2(ctx, descriptor)
	if err != nil {
		return nil, fmt.Errorf("unable to load schema from %s: %w", descriptor, err)
	}
	return pkg, nil

}

// ParameterizationAwareLoader wraps a schema.ReferenceLoader and enriches load
// requests for parameterized packages with the correct base provider name and
// parameterization.
type ParameterizationAwareLoader struct {
	inner   schema.ReferenceLoader
	aliases map[string]workspace.PackageDescriptor
}

func NewParameterizationAwareLoader(
	inner schema.ReferenceLoader,
	aliases map[string]workspace.PackageDescriptor,
) *ParameterizationAwareLoader {
	return &ParameterizationAwareLoader{inner: inner, aliases: aliases}
}

func (l *ParameterizationAwareLoader) enrich(descriptor *schema.PackageDescriptor) *schema.PackageDescriptor {
	if descriptor.Parameterization != nil {
		return descriptor
	}
	desc, ok := l.aliases[descriptor.Name]
	if !ok || desc.Parameterization == nil || desc.Version == nil {
		return descriptor
	}
	paramVersion := desc.Parameterization.Version
	baseVersion := *desc.Version
	return &schema.PackageDescriptor{
		Name:    desc.Name,
		Version: &baseVersion,
		Parameterization: &schema.ParameterizationDescriptor{
			Name:    desc.Parameterization.Name,
			Version: paramVersion,
			Value:   desc.Parameterization.Value,
		},
	}
}

func (l *ParameterizationAwareLoader) LoadPackage(pkg string, version *semver.Version) (*schema.Package, error) {
	return l.LoadPackageV2(context.TODO(), &schema.PackageDescriptor{Name: pkg, Version: version})
}

func (l *ParameterizationAwareLoader) LoadPackageV2(ctx context.Context, descriptor *schema.PackageDescriptor) (*schema.Package, error) {
	ref, err := l.LoadPackageReferenceV2(ctx, descriptor)
	if err != nil {
		return nil, err
	}
	return ref.Definition()
}

func (l *ParameterizationAwareLoader) LoadPackageReference(pkg string, version *semver.Version) (schema.PackageReference, error) {
	return l.LoadPackageReferenceV2(context.TODO(), &schema.PackageDescriptor{Name: pkg, Version: version})
}

func (l *ParameterizationAwareLoader) LoadPackageReferenceV2(ctx context.Context, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	return l.inner.LoadPackageReferenceV2(ctx, l.enrich(descriptor))
}

var _ schema.ReferenceLoader = (*ParameterizationAwareLoader)(nil)

func ResolveFunction(ctx context.Context, loader schema.ReferenceLoader, knownProviders []string, token string) (*schema.Function, error) {
	parts := strings.Split(token, "_")
	if len(parts) < 2 {
		return nil, InvalidToken{token: token, reason: "Pulumi HCL tokens must have at least 2 parts"}
	}

	pkgName, err := packageFromToken(knownProviders, token)
	if err != nil {
		return nil, err
	}

	pkg, err := resolvePackage(ctx, loader, &schema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, ErrNotFound
	}

	suffix := token[len(pkgName)+1:]
	suffixParts := strings.Split(suffix, "_")

	key := strings.ReplaceAll(suffix, "_", "")
	for iter := pkg.Functions().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.NewReplacer("/", "", "_", "").Replace(strings.ToLower(mod+name)) == key {
			return iter.Function()
		}
	}

	// Allow omitting the "get" on Pulumi datasources.
	implicitGetKey := suffixParts[0] + "get" + strings.Join(suffixParts[1:], "")
	for iter := pkg.Functions().Range(); iter.Next(); {
		mod := pkg.TokenToModule(iter.Token())
		name := strings.Split(iter.Token(), ":")[2]
		if strings.NewReplacer("/", "", "_", "").Replace(strings.ToLower(mod+name)) == implicitGetKey {
			return iter.Function()
		}
	}

	return nil, ErrNotFound
}
