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

package resolve

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeResolver answers ResolvePackage by dispatching on the spec's source.
type fakeResolver struct {
	respond func(*pulumirpc.PackageSpec) (*pulumirpc.PackageDependency, error)
}

func (f fakeResolver) ResolvePackage(
	_ context.Context, in *pulumirpc.PackageSpec, _ ...grpc.CallOption,
) (*pulumirpc.PackageDependency, error) {
	return f.respond(in)
}

func TestPackages(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{respond: func(spec *pulumirpc.PackageSpec) (*pulumirpc.PackageDependency, error) {
		switch spec.Source {
		case "aws":
			// A native pulumi package resolves to a plain plugin, no parameterization.
			return &pulumirpc.PackageDependency{
				Name:    "aws",
				Kind:    "resource",
				Version: "6.0.0",
			}, nil
		case "terraform-provider":
			// A bridged Terraform provider resolves to the terraform-provider plugin
			// plus the parameterization that names the produced package.
			return &pulumirpc.PackageDependency{
				Name:    "terraform-provider",
				Kind:    "resource",
				Version: "0.9.0",
				Parameterization: &pulumirpc.PackageParameterization{
					Name:    "random",
					Version: "3.6.0",
					Value:   []byte("hashicorp/random"),
				},
			}, nil
		default:
			return nil, errors.New("unexpected source: " + spec.Source)
		}
	}}

	got, err := Packages(context.Background(), resolver, []Request{
		{Alias: "aws", Spec: &pulumirpc.PackageSpec{Source: "aws", Version: "~> 6.0"}},
		{Alias: "random", Spec: &pulumirpc.PackageSpec{
			Source:     "terraform-provider",
			Parameters: []string{"hashicorp/random", "~> 3.0"},
		}},
	})
	require.NoError(t, err)

	awsVersion := semver.MustParse("6.0.0")
	tfVersion := semver.MustParse("0.9.0")
	assert.Equal(t, map[string]workspace.PackageDescriptor{
		"aws": {
			PluginDescriptor: workspace.PluginDescriptor{
				Name:    "aws",
				Kind:    apitype.ResourcePlugin,
				Version: &awsVersion,
			},
		},
		"random": {
			PluginDescriptor: workspace.PluginDescriptor{
				Name:    "terraform-provider",
				Kind:    apitype.ResourcePlugin,
				Version: &tfVersion,
			},
			Parameterization: &workspace.Parameterization{
				Name:    "random",
				Version: semver.MustParse("3.6.0"),
				Value:   []byte("hashicorp/random"),
			},
		},
	}, got)
}

func TestPackagesResolverError(t *testing.T) {
	t.Parallel()

	resolver := fakeResolver{respond: func(*pulumirpc.PackageSpec) (*pulumirpc.PackageDependency, error) {
		return nil, errors.New("boom")
	}}

	_, err := Packages(context.Background(), resolver, []Request{
		{Alias: "aws", Spec: &pulumirpc.PackageSpec{Source: "aws"}},
	})
	assert.EqualError(t, err, `resolving provider "aws": boom`)
}
