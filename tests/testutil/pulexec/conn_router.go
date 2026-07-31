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

package pulexec

import (
	"context"
	"sync"

	"github.com/pulumi/pulumi-hcl/tests/testutil/tfexec"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"
)

// connRoutedProvider is a pulumirpc.ResourceProviderServer that builds a
// separate provider instance (via start) for each gRPC client connection,
// identified by the id a tfexec.ConnTagger on the serving grpc.Server
// attached. The Pulumi engine dials the attached port once per provider
// resource instance, so routing by connection gives every instance its own
// bridged provider — matching production, where each instance is its own
// plugin process. A single shared instance would share configured state
// across instances (last Configure wins), coupling every resource to
// whichever instance configured last.
type connRoutedProvider struct {
	pulumirpc.UnimplementedResourceProviderServer

	start func(context.Context) (pulumirpc.ResourceProviderServer, error)

	mu     sync.Mutex
	byConn map[uint64]pulumirpc.ResourceProviderServer
}

var _ pulumirpc.ResourceProviderServer = (*connRoutedProvider)(nil)

func newConnRoutedProvider(
	start func(context.Context) (pulumirpc.ResourceProviderServer, error),
) *connRoutedProvider {
	return &connRoutedProvider{start: start, byConn: map[uint64]pulumirpc.ResourceProviderServer{}}
}

// delegate returns the provider instance owned by the calling connection,
// creating it on first use.
func (s *connRoutedProvider) delegate(ctx context.Context) (pulumirpc.ResourceProviderServer, error) {
	key := tfexec.ConnID(ctx)
	s.mu.Lock()
	defer s.mu.Unlock()
	srv, ok := s.byConn[key]
	if !ok {
		var err error
		srv, err = s.start(ctx)
		if err != nil {
			return nil, err
		}
		s.byConn[key] = srv
	}
	return srv, nil
}

func (s *connRoutedProvider) Handshake(
	ctx context.Context, req *pulumirpc.ProviderHandshakeRequest,
) (*pulumirpc.ProviderHandshakeResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Handshake(ctx, req)
}

func (s *connRoutedProvider) Parameterize(
	ctx context.Context, req *pulumirpc.ParameterizeRequest,
) (*pulumirpc.ParameterizeResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Parameterize(ctx, req)
}

func (s *connRoutedProvider) GetSchema(
	ctx context.Context, req *pulumirpc.GetSchemaRequest,
) (*pulumirpc.GetSchemaResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.GetSchema(ctx, req)
}

func (s *connRoutedProvider) CheckConfig(
	ctx context.Context, req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.CheckConfig(ctx, req)
}

func (s *connRoutedProvider) DiffConfig(
	ctx context.Context, req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.DiffConfig(ctx, req)
}

func (s *connRoutedProvider) Configure(
	ctx context.Context, req *pulumirpc.ConfigureRequest,
) (*pulumirpc.ConfigureResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Configure(ctx, req)
}

func (s *connRoutedProvider) Invoke(
	ctx context.Context, req *pulumirpc.InvokeRequest,
) (*pulumirpc.InvokeResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Invoke(ctx, req)
}

func (s *connRoutedProvider) Call(
	ctx context.Context, req *pulumirpc.CallRequest,
) (*pulumirpc.CallResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Call(ctx, req)
}

func (s *connRoutedProvider) Check(
	ctx context.Context, req *pulumirpc.CheckRequest,
) (*pulumirpc.CheckResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Check(ctx, req)
}

func (s *connRoutedProvider) Diff(
	ctx context.Context, req *pulumirpc.DiffRequest,
) (*pulumirpc.DiffResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Diff(ctx, req)
}

func (s *connRoutedProvider) Create(
	ctx context.Context, req *pulumirpc.CreateRequest,
) (*pulumirpc.CreateResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Create(ctx, req)
}

func (s *connRoutedProvider) Read(
	ctx context.Context, req *pulumirpc.ReadRequest,
) (*pulumirpc.ReadResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Read(ctx, req)
}

func (s *connRoutedProvider) Update(
	ctx context.Context, req *pulumirpc.UpdateRequest,
) (*pulumirpc.UpdateResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Update(ctx, req)
}

func (s *connRoutedProvider) Delete(
	ctx context.Context, req *pulumirpc.DeleteRequest,
) (*emptypb.Empty, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Delete(ctx, req)
}

func (s *connRoutedProvider) Construct(
	ctx context.Context, req *pulumirpc.ConstructRequest,
) (*pulumirpc.ConstructResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Construct(ctx, req)
}

func (s *connRoutedProvider) Cancel(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Cancel(ctx, req)
}

func (s *connRoutedProvider) GetPluginInfo(ctx context.Context, req *emptypb.Empty) (*pulumirpc.PluginInfo, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.GetPluginInfo(ctx, req)
}

func (s *connRoutedProvider) Attach(ctx context.Context, req *pulumirpc.PluginAttach) (*emptypb.Empty, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.Attach(ctx, req)
}

func (s *connRoutedProvider) GetMapping(
	ctx context.Context, req *pulumirpc.GetMappingRequest,
) (*pulumirpc.GetMappingResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.GetMapping(ctx, req)
}

func (s *connRoutedProvider) GetMappings(
	ctx context.Context, req *pulumirpc.GetMappingsRequest,
) (*pulumirpc.GetMappingsResponse, error) {
	d, err := s.delegate(ctx)
	if err != nil {
		return nil, err
	}
	return d.GetMappings(ctx, req)
}

func (s *connRoutedProvider) List(
	req *pulumirpc.ListRequest, stream grpc.ServerStreamingServer[pulumirpc.ListResponse],
) error {
	d, err := s.delegate(stream.Context())
	if err != nil {
		return err
	}
	return d.List(req, stream)
}
