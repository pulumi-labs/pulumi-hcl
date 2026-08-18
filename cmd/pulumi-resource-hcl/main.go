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

// pulumi-resource-hcl is the fully dynamic HCL resource provider. It serves the
// hcl:index:Module component resource, which instantiates a Terraform/OpenTofu
// module whose source is supplied at runtime, resolving the module's providers
// through the schema loader, mapper, and resolver the engine exposes at
// handshake.
package main

import (
	"context"
	"fmt"
	"os"

	p "github.com/pulumi/pulumi-go-provider"
	comProvider "github.com/pulumi/pulumi/sdk/v3/go/pulumi/provider"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"

	"github.com/pulumi/pulumi-hcl/pkg/server"
	"github.com/pulumi/pulumi-hcl/pkg/version"
)

func main() {
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Println(version.Version())
			os.Exit(0)
		}
	}

	ctx := context.Background()
	v := version.Version().String()
	// Expanded from p.RunProvider so the rpc server can be wrapped: spec-form
	// Construct aliases must be collapsed before pulumi-go-provider decodes
	// (and would drop) them.
	err := comProvider.MainContext(ctx, "hcl", func(host *comProvider.HostClient) (pulumirpc.ResourceProviderServer, error) {
		srv, err := p.RawServer("hcl", v, server.NewModuleProvider(ctx, v))(host)
		if err != nil {
			return nil, err
		}
		return server.WithCollapsedConstructAliases(srv), nil
	})
	if err != nil {
		fmt.Fprintf(os.Stderr, "fatal: %v\n", err)
		os.Exit(1)
	}
}
