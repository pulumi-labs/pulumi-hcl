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
	"context"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/require"
)

var _ schema.ReferenceLoader = mockReferenceLoader{}

type mockReferenceLoader map[string]schema.Package

func (m mockReferenceLoader) LoadPackage(pkg string, version *semver.Version) (*schema.Package, error) {
	return m.LoadPackageV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m mockReferenceLoader) LoadPackageV2(ctx context.Context, descriptor *schema.PackageDescriptor) (*schema.Package, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return &p, nil
	}
	return nil, ErrNotFound
}

func (m mockReferenceLoader) LoadPackageReference(pkg string, version *semver.Version) (schema.PackageReference, error) {
	return m.LoadPackageReferenceV2(context.Background(), &schema.PackageDescriptor{
		Name:    pkg,
		Version: version,
	})
}

func (m mockReferenceLoader) LoadPackageReferenceV2(ctx context.Context, descriptor *schema.PackageDescriptor) (schema.PackageReference, error) {
	p, ok := m[descriptor.String()]
	if ok {
		return p.Reference(), nil
	}
	return nil, ErrNotFound
}

func newTestLoader(t *testing.T, specs ...schema.PackageSpec) schema.ReferenceLoader {
	loader := mockReferenceLoader{}
	for _, spec := range specs {
		pkg, diag, err := schema.BindSpec(spec, loader, schema.ValidationOptions{})
		require.NoError(t, err)
		require.Empty(t, diag)
		d, err := pkg.Descriptor(context.Background())
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

func TestInvalidToken_Error(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		token    string
		reason   string
		expected string
	}{
		{
			name:     "token and reason",
			token:    "aws",
			reason:   "must have at least 2 parts",
			expected: `invalid token "aws" must have at least 2 parts`,
		},
		{
			name:     "token only",
			token:    "foo",
			reason:   "",
			expected: `invalid token "foo"`,
		},
		{
			name:     "reason only",
			token:    "",
			reason:   "some reason",
			expected: "invalid token some reason",
		},
		{
			name:     "neither",
			token:    "",
			reason:   "",
			expected: "invalid token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := InvalidToken{token: tt.token, reason: tt.reason}
			require.Equal(t, tt.expected, err.Error())
		})
	}
}

func TestResolveResource(t *testing.T) {
	t.Parallel()

	loader := newTestLoader(t,
		schema.PackageSpec{
			Name: "aws",
			Resources: map[string]schema.ResourceSpec{
				"aws:s3:Bucket":      {},
				"aws:index:Instance": {},
				"aws:ec2:Vpc":        {},
			},
		},
		schema.PackageSpec{
			Name: "gcp",
			Resources: map[string]schema.ResourceSpec{
				"gcp:storage:Bucket": {},
			},
		},
	)

	ctx := context.Background()

	tests := []struct {
		name         string
		token        string
		wantToken    string
		wantErr      error
		errAsInvalid bool
		errContains  string
	}{
		{
			name:      "basic resource",
			token:     "aws_s3_bucket",
			wantToken: "aws:s3:Bucket",
		},
		{
			name:      "index module",
			token:     "aws_instance",
			wantToken: "aws:index:Instance",
		},
		{
			name:      "multi-part module",
			token:     "aws_ec2_vpc",
			wantToken: "aws:ec2:Vpc",
		},
		{
			name:      "gcp provider",
			token:     "gcp_storage_bucket",
			wantToken: "gcp:storage:Bucket",
		},
		{
			name:         "single part token",
			token:        "aws",
			errAsInvalid: true,
			errContains:  "at least 2 parts",
		},
		{
			name:         "empty token",
			token:        "",
			errAsInvalid: true,
			errContains:  "at least 2 parts",
		},
		{
			name:    "resource not found",
			token:   "aws_nonexistent",
			wantErr: ErrNotFound,
		},
		{
			name:        "package not found",
			token:       "fake_resource",
			errContains: "unable to load schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ResolveResource(ctx, loader, tt.token)

			if tt.errAsInvalid {
				require.Error(t, err)
				var invalidToken InvalidToken
				require.ErrorAs(t, err, &invalidToken)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			if tt.errContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, res)
			actualToken := res.Token
			require.Equal(t, tt.wantToken, actualToken)
		})
	}
}

func TestResolveFunction(t *testing.T) {
	t.Parallel()

	loader := newTestLoader(t,
		schema.PackageSpec{
			Name: "aws",
			Functions: map[string]schema.FunctionSpec{
				"aws:s3:getBucket":      {},
				"aws:s3:listBuckets":    {},
				"aws:index:getInstance": {},
				"aws:ec2:getVpc":        {},
			},
		},
		schema.PackageSpec{
			Name: "gcp",
			Functions: map[string]schema.FunctionSpec{
				"gcp:storage:getBucket": {},
			},
		},
	)

	ctx := context.Background()

	tests := []struct {
		name         string
		token        string
		wantToken    string
		wantErr      error
		errAsInvalid bool
		errContains  string
	}{
		{
			name:      "direct function match",
			token:     "aws_s3_getbucket",
			wantToken: "aws:s3:getBucket",
		},
		{
			name:      "index module function",
			token:     "aws_getinstance",
			wantToken: "aws:index:getInstance",
		},
		{
			name:      "implicit get prefix",
			token:     "aws_s3_bucket",
			wantToken: "aws:s3:getBucket",
		},
		{
			name:      "implicit get prefix multi-part",
			token:     "aws_ec2_vpc",
			wantToken: "aws:ec2:getVpc",
		},
		{
			name:      "gcp implicit get",
			token:     "gcp_storage_bucket",
			wantToken: "gcp:storage:getBucket",
		},
		{
			name:      "list function",
			token:     "aws_s3_listbuckets",
			wantToken: "aws:s3:listBuckets",
		},
		{
			name:         "single part token",
			token:        "aws",
			errAsInvalid: true,
			errContains:  "at least 2 parts",
		},
		{
			name:         "empty token",
			token:        "",
			errAsInvalid: true,
			errContains:  "at least 2 parts",
		},
		{
			name:    "function not found",
			token:   "aws_nonexistent",
			wantErr: ErrNotFound,
		},
		{
			name:        "package not found",
			token:       "fake_function",
			errContains: "unable to load schema",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := ResolveFunction(ctx, loader, tt.token)

			if tt.errAsInvalid {
				require.Error(t, err)
				var invalidToken InvalidToken
				require.ErrorAs(t, err, &invalidToken)
				if tt.errContains != "" {
					require.Contains(t, err.Error(), tt.errContains)
				}
				return
			}

			if tt.wantErr != nil {
				require.ErrorIs(t, err, tt.wantErr)
				return
			}

			if tt.errContains != "" {
				require.Error(t, err)
				require.Contains(t, err.Error(), tt.errContains)
				return
			}

			require.NoError(t, err)
			require.NotNil(t, fn)
			actualToken := fn.Token
			require.Equal(t, tt.wantToken, actualToken)
		})
	}
}
