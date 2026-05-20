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

package packages

import (
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/schemaloader"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/codegen/testing/utils/rapidschema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"pgregory.net/rapid"
)

func TestNearestHCLToken(t *testing.T) {
	t.Parallel()

	loader := schemaloader.New(t, schema.PackageSpec{
		Name: "aws",
		Meta: &schema.MetadataSpec{
			ModuleFormat: `(.*)(?:/[^/]*)`,
		},
		Resources: map[string]schema.ResourceSpec{
			"aws:ec2/vpc:Vpc":                  {},
			"aws:s3/bucket:Bucket":             {},
			"aws:lb/loadBalancer:LoadBalancer": {},
		},
		Functions: map[string]schema.FunctionSpec{
			"aws:ec2/getVpc:getVpc":                             {},
			"aws:index/getAvailabilityZone:getAvailabilityZone": {},
		},
	})
	pkg, err := loader.LoadPackageReferenceV2(t.Context(), &schema.PackageDescriptor{Name: "aws"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		hclToken   string
		isFunction bool
		want       string
	}{
		{"one-character typo on resource", "aws_ec2_vpd", false, "aws_ec2_vpc"},
		{"camelCase expansion", "aws_lb_load_balancr", false, "aws_lb_load_balancer"},
		{"unrelated returns empty", "aws_completely_unrelated_thing_xyz", false, ""},
		{"function typo strips get", "aws_availability_zonee", true, "aws_availability_zone"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := nearestHCLToken(pkg, tt.hclToken, tt.isFunction)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPulumiTokenToHCLForm(t *testing.T) {
	t.Parallel()

	loader := schemaloader.New(t,
		schema.PackageSpec{
			Name: "aws",
			Meta: &schema.MetadataSpec{
				ModuleFormat: `(.*)(?:/[^/]*)`,
			},
			Resources: map[string]schema.ResourceSpec{
				"aws:ec2/vpc:Vpc":                  {},
				"aws:lb/loadBalancer:LoadBalancer": {},
				"aws:index/instance:Instance":      {},
			},
		},
	)
	pkg, err := loader.LoadPackageReferenceV2(t.Context(), &schema.PackageDescriptor{Name: "aws"})
	require.NoError(t, err)

	tests := []struct {
		name       string
		token      string
		isFunction bool
		want       string
	}{
		{"resource with snake-case name", "aws:ec2/vpc:Vpc", false, "aws_ec2_vpc"},
		{"resource with camelCase name", "aws:lb/loadBalancer:LoadBalancer", false, "aws_lb_load_balancer"},
		{"index module omitted", "aws:index/instance:Instance", false, "aws_instance"},
		{"function strips get prefix", "aws:ec2/getVpc:getVpc", true, "aws_ec2_vpc"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := pulumiTokenToHCLForm(pkg, tt.token, tt.isFunction)
			require.Equal(t, tt.want, got)
		})
	}
}

func TestPulumiTokenToHCLFormRoundtrip(t *testing.T) {
	t.Parallel()

	rapid.Check(t, func(t *rapid.T) {
		pkg := rapidschema.Package().Draw(t, "pkg")
		spec, err := pkg.MarshalSpec()
		require.NoError(t, err)
		for _, r := range pkg.Resources {
			hclTk := pulumiTokenToHCLForm(pkg.Reference(), r.Token, false)
			resolved, err := ResolveResource(t.Context(), schemaloader.New(t, *spec), nil, hclTk)
			require.NoError(t, err)
			assert.Equal(t, r, resolved)
		}
	})
}

func TestLevenshtein(t *testing.T) {
	t.Parallel()

	tests := []struct {
		a, b string
		want int
	}{
		{"", "", 0},
		{"abc", "abc", 0},
		{"abc", "abd", 1},
		{"abc", "abcd", 1},
		{"abc", "", 3},
		{"kitten", "sitting", 3},
		{"aws_ec2_vpd", "aws_ec2_vpc", 1},
	}

	for _, tt := range tests {
		got := levenshtein(tt.a, tt.b)
		require.Equal(t, tt.want, got, "levenshtein(%q, %q)", tt.a, tt.b)
	}
}
