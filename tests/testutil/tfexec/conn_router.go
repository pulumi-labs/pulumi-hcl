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

package tfexec

import (
	"context"
	"sync"

	"github.com/hashicorp/terraform-plugin-go/tfprotov6"
)

// ConnRoutedServer returns a tfprotov6.ProviderServer that builds a separate
// provider instance (via factory) for each gRPC client connection, identified
// by the id a ConnTagger on the serving grpc.Server attached. OpenTofu dials
// the reattach server once per configured provider instance, so routing by
// connection gives every instance its own provider — matching production,
// where each instance is its own plugin process. A single shared instance
// would share configured meta across instances (last Configure wins),
// coupling every resource to whichever instance configured last.
func ConnRoutedServer(factory func() tfprotov6.ProviderServer) tfprotov6.ProviderServer {
	return &connRoutedServer{factory: factory, byConn: map[uint64]tfprotov6.ProviderServer{}}
}

type connRoutedServer struct {
	factory func() tfprotov6.ProviderServer

	mu     sync.Mutex
	byConn map[uint64]tfprotov6.ProviderServer
}

var _ tfprotov6.ProviderServer = (*connRoutedServer)(nil)

// delegate returns the provider instance owned by the calling connection,
// creating it on first use.
func (s *connRoutedServer) delegate(ctx context.Context) tfprotov6.ProviderServer {
	key := ConnID(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	srv, ok := s.byConn[key]
	if !ok {
		srv = s.factory()
		s.byConn[key] = srv
	}
	return srv
}

func (s *connRoutedServer) GetMetadata(
	ctx context.Context, req *tfprotov6.GetMetadataRequest,
) (*tfprotov6.GetMetadataResponse, error) {
	return s.delegate(ctx).GetMetadata(ctx, req)
}

func (s *connRoutedServer) GetProviderSchema(
	ctx context.Context, req *tfprotov6.GetProviderSchemaRequest,
) (*tfprotov6.GetProviderSchemaResponse, error) {
	return s.delegate(ctx).GetProviderSchema(ctx, req)
}

func (s *connRoutedServer) GetResourceIdentitySchemas(
	ctx context.Context, req *tfprotov6.GetResourceIdentitySchemasRequest,
) (*tfprotov6.GetResourceIdentitySchemasResponse, error) {
	return s.delegate(ctx).GetResourceIdentitySchemas(ctx, req)
}

func (s *connRoutedServer) ValidateProviderConfig(
	ctx context.Context, req *tfprotov6.ValidateProviderConfigRequest,
) (*tfprotov6.ValidateProviderConfigResponse, error) {
	return s.delegate(ctx).ValidateProviderConfig(ctx, req)
}

func (s *connRoutedServer) ConfigureProvider(
	ctx context.Context, req *tfprotov6.ConfigureProviderRequest,
) (*tfprotov6.ConfigureProviderResponse, error) {
	return s.delegate(ctx).ConfigureProvider(ctx, req)
}

func (s *connRoutedServer) StopProvider(
	ctx context.Context, req *tfprotov6.StopProviderRequest,
) (*tfprotov6.StopProviderResponse, error) {
	return s.delegate(ctx).StopProvider(ctx, req)
}

func (s *connRoutedServer) ValidateResourceConfig(
	ctx context.Context, req *tfprotov6.ValidateResourceConfigRequest,
) (*tfprotov6.ValidateResourceConfigResponse, error) {
	return s.delegate(ctx).ValidateResourceConfig(ctx, req)
}

func (s *connRoutedServer) UpgradeResourceState(
	ctx context.Context, req *tfprotov6.UpgradeResourceStateRequest,
) (*tfprotov6.UpgradeResourceStateResponse, error) {
	return s.delegate(ctx).UpgradeResourceState(ctx, req)
}

func (s *connRoutedServer) UpgradeResourceIdentity(
	ctx context.Context, req *tfprotov6.UpgradeResourceIdentityRequest,
) (*tfprotov6.UpgradeResourceIdentityResponse, error) {
	return s.delegate(ctx).UpgradeResourceIdentity(ctx, req)
}

func (s *connRoutedServer) ReadResource(
	ctx context.Context, req *tfprotov6.ReadResourceRequest,
) (*tfprotov6.ReadResourceResponse, error) {
	return s.delegate(ctx).ReadResource(ctx, req)
}

func (s *connRoutedServer) PlanResourceChange(
	ctx context.Context, req *tfprotov6.PlanResourceChangeRequest,
) (*tfprotov6.PlanResourceChangeResponse, error) {
	return s.delegate(ctx).PlanResourceChange(ctx, req)
}

func (s *connRoutedServer) ApplyResourceChange(
	ctx context.Context, req *tfprotov6.ApplyResourceChangeRequest,
) (*tfprotov6.ApplyResourceChangeResponse, error) {
	return s.delegate(ctx).ApplyResourceChange(ctx, req)
}

func (s *connRoutedServer) ImportResourceState(
	ctx context.Context, req *tfprotov6.ImportResourceStateRequest,
) (*tfprotov6.ImportResourceStateResponse, error) {
	return s.delegate(ctx).ImportResourceState(ctx, req)
}

func (s *connRoutedServer) MoveResourceState(
	ctx context.Context, req *tfprotov6.MoveResourceStateRequest,
) (*tfprotov6.MoveResourceStateResponse, error) {
	return s.delegate(ctx).MoveResourceState(ctx, req)
}

func (s *connRoutedServer) GenerateResourceConfig(
	ctx context.Context, req *tfprotov6.GenerateResourceConfigRequest,
) (*tfprotov6.GenerateResourceConfigResponse, error) {
	return s.delegate(ctx).GenerateResourceConfig(ctx, req)
}

func (s *connRoutedServer) ValidateDataResourceConfig(
	ctx context.Context, req *tfprotov6.ValidateDataResourceConfigRequest,
) (*tfprotov6.ValidateDataResourceConfigResponse, error) {
	return s.delegate(ctx).ValidateDataResourceConfig(ctx, req)
}

func (s *connRoutedServer) ReadDataSource(
	ctx context.Context, req *tfprotov6.ReadDataSourceRequest,
) (*tfprotov6.ReadDataSourceResponse, error) {
	return s.delegate(ctx).ReadDataSource(ctx, req)
}

func (s *connRoutedServer) ValidateEphemeralResourceConfig(
	ctx context.Context, req *tfprotov6.ValidateEphemeralResourceConfigRequest,
) (*tfprotov6.ValidateEphemeralResourceConfigResponse, error) {
	return s.delegate(ctx).ValidateEphemeralResourceConfig(ctx, req)
}

func (s *connRoutedServer) OpenEphemeralResource(
	ctx context.Context, req *tfprotov6.OpenEphemeralResourceRequest,
) (*tfprotov6.OpenEphemeralResourceResponse, error) {
	return s.delegate(ctx).OpenEphemeralResource(ctx, req)
}

func (s *connRoutedServer) RenewEphemeralResource(
	ctx context.Context, req *tfprotov6.RenewEphemeralResourceRequest,
) (*tfprotov6.RenewEphemeralResourceResponse, error) {
	return s.delegate(ctx).RenewEphemeralResource(ctx, req)
}

func (s *connRoutedServer) CloseEphemeralResource(
	ctx context.Context, req *tfprotov6.CloseEphemeralResourceRequest,
) (*tfprotov6.CloseEphemeralResourceResponse, error) {
	return s.delegate(ctx).CloseEphemeralResource(ctx, req)
}

func (s *connRoutedServer) CallFunction(
	ctx context.Context, req *tfprotov6.CallFunctionRequest,
) (*tfprotov6.CallFunctionResponse, error) {
	return s.delegate(ctx).CallFunction(ctx, req)
}

func (s *connRoutedServer) GetFunctions(
	ctx context.Context, req *tfprotov6.GetFunctionsRequest,
) (*tfprotov6.GetFunctionsResponse, error) {
	return s.delegate(ctx).GetFunctions(ctx, req)
}
