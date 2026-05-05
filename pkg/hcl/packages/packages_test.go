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
	"github.com/stretchr/testify/require"
)

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

	loader := schemaloader.New(t,
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
		schema.PackageSpec{
			Name: "fail_on_create",
			Resources: map[string]schema.ResourceSpec{
				"fail_on_create:index:Resource": {},
			},
		},
	)

	ctx := t.Context()

	tests := []struct {
		name           string
		knownProviders []string
		token          string
		wantToken      string
		wantErr        error
		errAsInvalid   bool
		errContains    string
	}{
		{
			name:           "basic resource",
			knownProviders: []string{"aws"},
			token:          "aws_s3_bucket",
			wantToken:      "aws:s3:Bucket",
		},
		{
			name:           "index module",
			knownProviders: []string{"aws"},
			token:          "aws_instance",
			wantToken:      "aws:index:Instance",
		},
		{
			name:           "multi-part module",
			knownProviders: []string{"aws"},
			token:          "aws_ec2_vpc",
			wantToken:      "aws:ec2:Vpc",
		},
		{
			name:           "gcp provider",
			knownProviders: []string{"gcp"},
			token:          "gcp_storage_bucket",
			wantToken:      "gcp:storage:Bucket",
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
			name:           "resource not found",
			knownProviders: []string{"aws"},
			token:          "aws_nonexistent",
			wantErr:        ErrNotFound,
		},
		{
			name:    "package not found",
			token:   "fake_resource",
			wantErr: ErrNotFound,
		},
		{
			name:           "underscore package name",
			knownProviders: []string{"fail_on_create", "simple"},
			token:          "fail_on_create_resource",
			wantToken:      "fail_on_create:index:Resource",
		},
		{
			name:           "ambiguous token",
			knownProviders: []string{"foo", "foo_bar"},
			token:          "foo_bar_thing",
			errContains:    "ambiguous token",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			res, err := ResolveResource(ctx, loader, tt.knownProviders, tt.token)

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

	loader := schemaloader.New(t,
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
		schema.PackageSpec{
			Name: "mypkg",
			Meta: &schema.MetadataSpec{
				// Non-standard module format where the module is everything before the last _name segment.
				ModuleFormat: `(.*)(?:_[^_]*)`,
			},
			Functions: map[string]schema.FunctionSpec{
				"mypkg:mod_concatWorld:concatWorld":        {},
				"mypkg:mod/nested_concatWorld:concatWorld": {},
			},
		},
	)

	ctx := t.Context()

	tests := []struct {
		name           string
		knownProviders []string
		token          string
		wantToken      string
		wantErr        error
		errAsInvalid   bool
		errContains    string
	}{
		{
			name:           "direct function match",
			knownProviders: []string{"aws"},
			token:          "aws_s3_getbucket",
			wantToken:      "aws:s3:getBucket",
		},
		{
			name:           "index module function",
			knownProviders: []string{"aws"},
			token:          "aws_getinstance",
			wantToken:      "aws:index:getInstance",
		},
		{
			name:           "implicit get prefix",
			knownProviders: []string{"aws"},
			token:          "aws_s3_bucket",
			wantToken:      "aws:s3:getBucket",
		},
		{
			name:           "implicit get prefix multi-part",
			knownProviders: []string{"aws"},
			token:          "aws_ec2_vpc",
			wantToken:      "aws:ec2:getVpc",
		},
		{
			name:           "gcp implicit get",
			knownProviders: []string{"gcp"},
			token:          "gcp_storage_bucket",
			wantToken:      "gcp:storage:getBucket",
		},
		{
			name:           "list function",
			knownProviders: []string{"aws"},
			token:          "aws_s3_listbuckets",
			wantToken:      "aws:s3:listBuckets",
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
			name:           "non-standard module format",
			knownProviders: []string{"mypkg"},
			token:          "mypkg_mod_concatworld",
			wantToken:      "mypkg:mod_concatWorld:concatWorld",
		},
		{
			name:           "non-standard module format with nested slash",
			knownProviders: []string{"mypkg"},
			token:          "mypkg_mod_nested_concatworld",
			wantToken:      "mypkg:mod/nested_concatWorld:concatWorld",
		},
		{
			name:           "function not found",
			knownProviders: []string{"aws"},
			token:          "aws_nonexistent",
			wantErr:        ErrNotFound,
		},
		{
			name:    "package not found",
			token:   "fake_function",
			wantErr: ErrNotFound,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			fn, err := ResolveFunction(ctx, loader, tt.knownProviders, tt.token)

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
