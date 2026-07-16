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

package server

import (
	"context"
	"errors"
	"fmt"
	"testing"

	p "github.com/pulumi/pulumi-go-provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
)

func TestWithStatus(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name     string
		err      error
		fallback codes.Code
		want     codes.Code
	}{
		{
			name:     "module not found",
			err:      fmt.Errorf("loading module: %w", modules.ErrNotFound),
			fallback: codes.Unknown,
			want:     codes.NotFound,
		},
		{
			name:     "registry unauthenticated",
			err:      fmt.Errorf("loading module: %w", modules.ErrUnauthenticated),
			fallback: codes.Unknown,
			want:     codes.Unauthenticated,
		},
		{
			name:     "registry permission denied",
			err:      fmt.Errorf("loading module: %w", modules.ErrPermissionDenied),
			fallback: codes.Unknown,
			want:     codes.PermissionDenied,
		},
		{
			name:     "transient registry failure",
			err:      fmt.Errorf("loading module: %w", modules.ErrTransient),
			fallback: codes.Unknown,
			want:     codes.Unavailable,
		},
		{
			name:     "invalid source",
			err:      fmt.Errorf("loading module: %w", modules.ErrInvalid),
			fallback: codes.Unknown,
			want:     codes.InvalidArgument,
		},
		{
			name:     "unresolvable provider type",
			err:      fmt.Errorf("resolving resource: %w", &packages.NotFoundError{Token: "aws_foo"}),
			fallback: codes.Unknown,
			want:     codes.Unimplemented,
		},
		{
			name:     "context canceled",
			err:      fmt.Errorf("loading module: %w", context.Canceled),
			fallback: codes.Unknown,
			want:     codes.Canceled,
		},
		{
			name:     "context deadline exceeded",
			err:      fmt.Errorf("loading module: %w", context.DeadlineExceeded),
			fallback: codes.Unknown,
			want:     codes.DeadlineExceeded,
		},
		{
			name:     "existing status wins over fallback",
			err:      fmt.Errorf("generating schema: %w", errorf(codes.InvalidArgument, "bad args")),
			fallback: codes.Unimplemented,
			want:     codes.InvalidArgument,
		},
		{
			name:     "downstream rpc status kept",
			err:      fmt.Errorf("loading schema: %w", status.Error(codes.Unavailable, "engine gone")),
			fallback: codes.Unknown,
			want:     codes.Unavailable,
		},
		{
			name:     "unclassified uses fallback",
			err:      errors.New("boom"),
			fallback: codes.Unimplemented,
			want:     codes.Unimplemented,
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := withStatus(tc.err, tc.fallback)
			require.Equal(t, tc.want, status.Code(err))
			require.Equal(t, tc.err.Error(), err.Error(), "classification must not alter the message")
		})
	}

	require.NoError(t, withStatus(nil, codes.Unknown))
}

// TestStatusSurvivesWrapping guards the wire contract: grpc-go's
// status.FromError must recover the code from a statusError buried in a
// fmt.Errorf chain, and rebuild the message from the outermost error.
func TestStatusSurvivesWrapping(t *testing.T) {
	t.Parallel()
	err := fmt.Errorf("generating schema: %w", errorf(codes.Unimplemented, "cannot type local"))
	s, ok := status.FromError(err)
	require.True(t, ok)
	require.Equal(t, codes.Unimplemented, s.Code())
	require.Equal(t, "generating schema: cannot type local", s.Message())
}

// nopResolver is a non-nil PackageResolverClient so parameterize gets past its
// handshake check without dialing anything.
type nopResolver struct {
	pulumirpc.PackageResolverClient
}

func TestParameterizeArgsStatusCodes(t *testing.T) {
	t.Parallel()
	cases := []struct {
		name string
		args []string
		want codes.Code
	}{
		{"missing module keyword", []string{"not-module"}, codes.InvalidArgument},
		{"no args", nil, codes.InvalidArgument},
		{"too many args", []string{"module", "a", "b", "c"}, codes.InvalidArgument},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			m := &moduleProvider{resolver: nopResolver{}}
			_, err := m.parameterize(t.Context(), p.ParameterizeRequest{
				Args: &p.ParameterizeRequestArgs{Args: tc.args},
			})
			require.Equal(t, tc.want, status.Code(err))
		})
	}
}

func TestParameterizeBeforeHandshakeIsFailedPrecondition(t *testing.T) {
	t.Parallel()
	m := &moduleProvider{}
	_, err := m.parameterize(t.Context(), p.ParameterizeRequest{
		Args: &p.ParameterizeRequestArgs{Args: []string{"module", "acme/thing/aws"}},
	})
	require.Equal(t, codes.FailedPrecondition, status.Code(err))
}
