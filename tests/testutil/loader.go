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

package testutil

import (
	"context"
	"fmt"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"
)

var _ schema.ReferenceLoader = MockReferenceLoader{}

// MockReferenceLoader is a mock implementation of schema.ReferenceLoader for testing.
type MockReferenceLoader map[string]schema.Package

func (m MockReferenceLoader) LoadPackage(pkg string, version *semver.Version) (*schema.Package, error) {
	return m.LoadPackageV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m MockReferenceLoader) LoadPackageV2(ctx context.Context, descriptor *schema.PackageDescriptor) (*schema.Package, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return &p, nil
	}
	if p, ok := m.findByName(descriptor.Name); ok {
		return &p, nil
	}
	return nil, packages.ErrNotFound
}

func (m MockReferenceLoader) LoadPackageReference(pkg string, version *semver.Version) (schema.PackageReference, error) {
	return m.LoadPackageReferenceV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m MockReferenceLoader) LoadPackageReferenceV2(ctx context.Context, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return p.Reference(), nil
	}
	if p, ok := m.findByName(descriptor.Name); ok {
		return p.Reference(), nil
	}
	fmt.Printf("Looking for %s\n", descriptor.String())
	for k := range m {
		fmt.Printf("Found: %s\n", k)
	}
	return nil, packages.ErrNotFound
}

func (m MockReferenceLoader) findByName(name string) (schema.Package, bool) {
	for _, p := range m {
		if p.Name == name {
			return p, true
		}
	}
	return schema.Package{}, false
}

// NewMockReferenceLoader creates a MockReferenceLoader from the given PackageSpecs.
func NewMockReferenceLoader(t testing.TB, schemas ...schema.PackageSpec) schema.ReferenceLoader {
	loader := MockReferenceLoader{}
	for _, spec := range schemas {
		pkg, diag, err := schema.BindSpec(spec, loader, schema.ValidationOptions{})
		require.NoError(t, err)
		require.Len(t, diag, 0)
		d, err := pkg.Descriptor(t.Context())
		require.NoError(t, err)

		params := func() *schema.ParameterizationDescriptor {
			if d.Parameterization == nil {
				return nil
			}
			return &schema.ParameterizationDescriptor{
				Name:    d.Parameterization.Name,
				Version: d.Parameterization.Version,
				Value:   d.Parameterization.Value,
			}
		}
		loader[(&schema.PackageDescriptor{
			Name:             d.Name,
			Version:          d.Version,
			DownloadURL:      d.PluginDownloadURL,
			Parameterization: params(),
		}).String()] = *pkg
	}
	return loader
}
