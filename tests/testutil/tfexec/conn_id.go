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
	"sync/atomic"

	"google.golang.org/grpc/stats"
)

type connIDKey struct{}

// ConnTagger is a grpc stats.Handler that tags every server connection with a
// unique id, inherited by the context of each RPC on that connection. Routers
// read it back with ConnID to give each client connection its own provider
// instance. Connection identity cannot come from the gRPC peer address:
// go-plugin serves reattach providers on a unix socket, where every
// connection reports the same empty address.
type ConnTagger struct {
	nextID atomic.Uint64
}

var _ stats.Handler = (*ConnTagger)(nil)

func (t *ConnTagger) TagConn(ctx context.Context, _ *stats.ConnTagInfo) context.Context {
	return context.WithValue(ctx, connIDKey{}, t.nextID.Add(1))
}

func (*ConnTagger) HandleConn(context.Context, stats.ConnStats) {}

func (*ConnTagger) TagRPC(ctx context.Context, _ *stats.RPCTagInfo) context.Context { return ctx }

func (*ConnTagger) HandleRPC(context.Context, stats.RPCStats) {}

// ConnID returns the connection id attached by ConnTagger, or 0 when the
// server was built without one.
func ConnID(ctx context.Context) uint64 {
	id, _ := ctx.Value(connIDKey{}).(uint64)
	return id
}
