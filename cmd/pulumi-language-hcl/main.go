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

// pulumi-language-hcl is the Pulumi language host for HCL (HashiCorp Configuration Language).
// It implements a gRPC server that the Pulumi engine uses to execute HCL programs.
package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"os/signal"
	"time"

	"github.com/pulumi-labs/pulumi-hcl/pkg/server"
	"github.com/pulumi-labs/pulumi-hcl/pkg/version"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/cmdutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
)

type runParams struct {
	engineAddress string
	tracing       string
}

func parseRunParams(fs *flag.FlagSet, args []string) (*runParams, error) {
	var p runParams
	fs.StringVar(&p.tracing, "tracing", "", "Emit tracing to a Zipkin-compatible tracing endpoint")

	if err := fs.Parse(args); err != nil {
		return nil, err
	}

	// The engine address is passed as a positional argument
	args = fs.Args()
	if len(args) == 0 {
		fmt.Fprintln(os.Stderr, "Warning: launching without arguments, only for debugging")
	} else {
		p.engineAddress = args[0]
	}

	return &p, nil
}

func main() {
	// Check for --version before full parsing to avoid warning
	for _, arg := range os.Args[1:] {
		if arg == "--version" || arg == "-version" {
			fmt.Println(version.GetVersion())
			os.Exit(0)
		}
	}

	p, err := parseRunParams(flag.CommandLine, os.Args[1:])
	if err != nil {
		cmdutil.Exit(err)
	}

	logging.InitLogging(false, 0, false)
	if p.tracing != "" {
		cmdutil.InitTracing("pulumi-language-hcl", "pulumi-language-hcl", p.tracing)
	}

	if err := run(p); err != nil {
		cmdutil.Exit(err)
	}
}

func run(p *runParams) error {
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	// Map the context Done channel to a boolean cancel channel for rpcutil
	cancelChannel := make(chan bool)
	go func() {
		<-ctx.Done()
		cancel()
		close(cancelChannel)
	}()

	// If an engine address was provided, wait for it to be healthy
	if p.engineAddress != "" {
		err := rpcutil.Healthcheck(ctx, p.engineAddress, 5*time.Minute, cancel)
		if err != nil {
			return fmt.Errorf("could not connect to engine: %w", err)
		}
	}

	var host *server.LanguageHost
	defer func() {
		if host != nil {
			contract.IgnoreClose(host)
		}
	}()
	// Create the gRPC server
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancelChannel,
		Init: func(srv *grpc.Server) error {
			var err error
			host, err = server.NewLanguageHost(p.engineAddress)
			if err != nil {
				return err
			}
			pulumirpc.RegisterLanguageRuntimeServer(srv, host)
			return nil
		},
		Options: rpcutil.OpenTracingServerInterceptorOptions(nil),
	})
	if err != nil {
		return fmt.Errorf("could not start language host RPC server: %w", err)
	}

	// Print the port so the engine can connect
	fmt.Printf("%d\n", handle.Port)

	// Wait for the server to stop
	if err := <-handle.Done; err != nil {
		return fmt.Errorf("language host RPC stopped serving: %w", err)
	}

	return nil
}
