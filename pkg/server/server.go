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

// Package server implements the Pulumi language runtime gRPC server for HCL.
package server

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi-language-hcl/pkg/version"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/types/known/emptypb"
)

// LanguageHost implements the LanguageRuntimeServer gRPC interface.
type LanguageHost struct {
	pulumirpc.UnimplementedLanguageRuntimeServer
}

// NewLanguageHost creates a new HCL language host.
func NewLanguageHost() (*LanguageHost, error) {
	return &LanguageHost{}, nil
}

// GetRequiredPlugins returns the plugins required to run an HCL program.
func (host *LanguageHost) GetRequiredPlugins(
	ctx context.Context,
	req *pulumirpc.GetRequiredPluginsRequest,
) (*pulumirpc.GetRequiredPluginsResponse, error) {
	logging.V(5).Infof("GetRequiredPlugins: program=%s", req.Program)

	// Parse HCL files to extract required providers from terraform block
	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Program)
	if diags.HasErrors() {
		// Return empty list on parse errors - we'll report them during Run
		return &pulumirpc.GetRequiredPluginsResponse{
			Plugins: []*pulumirpc.PluginDependency{},
		}, nil
	}

	var plugins []*pulumirpc.PluginDependency

	// Extract required providers from the terraform block
	if config.Terraform != nil {
		for name, provider := range config.Terraform.RequiredProviders {
			dep := &pulumirpc.PluginDependency{
				Name: name,
				Kind: "resource",
			}

			// Parse version constraint if present
			// Pulumi expects semver, not constraints, so extract the version number
			if provider.Version != "" {
				dep.Version = extractSemverFromConstraint(provider.Version)
			}

			// Parse source to get the actual provider name
			// Format is typically "pulumi/aws" or "hashicorp/aws"
			if provider.Source != "" {
				parts := strings.Split(provider.Source, "/")
				if len(parts) >= 2 {
					dep.Name = parts[len(parts)-1]
				}
			}

			plugins = append(plugins, dep)
		}
	}

	return &pulumirpc.GetRequiredPluginsResponse{
		Plugins: plugins,
	}, nil
}

// Run executes an HCL program.
func (host *LanguageHost) Run(
	ctx context.Context,
	req *pulumirpc.RunRequest,
) (*pulumirpc.RunResponse, error) {
	logging.V(5).Infof("Run: program=%s, pwd=%s, stack=%s, project=%s",
		req.Info.EntryPoint, req.Pwd, req.Stack, req.Project)

	// Connect to the resource monitor
	conn, err := grpc.Dial(
		req.MonitorAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		rpcutil.GrpcChannelOptions(),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to resource monitor: %w", err)
	}
	defer conn.Close()

	// Create the resource monitor client
	monitorClient := pulumirpc.NewResourceMonitorClient(conn)
	resmon := &resourceMonitorAdapter{
		client: monitorClient,
		ctx:    ctx,
	}

	// Parse the HCL program
	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Program)
	if diags.HasErrors() {
		return &pulumirpc.RunResponse{
			Error: diags.Error(),
		}, nil
	}

	// Build config map from request
	configMap := make(map[string]string)
	for k, v := range req.Config {
		configMap[k] = v
	}

	schemaLoader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("unable to aquire gRPC schema loader: %w", err)
	}

	// Create and run the engine
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:      req.Project,
		StackName:        req.Stack,
		Config:           configMap,
		ConfigSecretKeys: req.ConfigSecretKeys,
		DryRun:           req.DryRun,
		ResourceMonitor:  resmon,
		SchemaLoader:     schema.NewCachedLoader(schemaLoader),
		WorkDir:          req.Info.ProgramDirectory,
	})

	if err := engine.Run(ctx); err != nil {
		return &pulumirpc.RunResponse{
			Error: err.Error(),
		}, nil
	}

	return &pulumirpc.RunResponse{}, nil
}

// GetPluginInfo returns information about this language plugin.
func (host *LanguageHost) GetPluginInfo(
	ctx context.Context,
	req *emptypb.Empty,
) (*pulumirpc.PluginInfo, error) {
	v := version.GetVersion()
	return &pulumirpc.PluginInfo{
		Version: v.String(),
	}, nil
}

// InstallDependencies installs dependencies for an HCL program.
func (host *LanguageHost) InstallDependencies(
	req *pulumirpc.InstallDependenciesRequest,
	server pulumirpc.LanguageRuntime_InstallDependenciesServer,
) error {
	// HCL programs don't have traditional package dependencies like Node.js or Python.
	// Provider plugins are managed by Pulumi automatically.
	return nil
}

// RuntimeOptionsPrompts returns prompts for runtime options during `pulumi new`.
func (host *LanguageHost) RuntimeOptionsPrompts(
	ctx context.Context,
	req *pulumirpc.RuntimeOptionsRequest,
) (*pulumirpc.RuntimeOptionsResponse, error) {
	return &pulumirpc.RuntimeOptionsResponse{
		Prompts: []*pulumirpc.RuntimeOptionPrompt{},
	}, nil
}

// About returns information about the HCL runtime.
func (host *LanguageHost) About(
	ctx context.Context,
	req *pulumirpc.AboutRequest,
) (*pulumirpc.AboutResponse, error) {
	return &pulumirpc.AboutResponse{
		Executable: "pulumi-language-hcl",
		Version:    version.GetVersion().String(),
	}, nil
}

// GetProgramDependencies returns the dependencies of an HCL program.
func (host *LanguageHost) GetProgramDependencies(
	ctx context.Context,
	req *pulumirpc.GetProgramDependenciesRequest,
) (*pulumirpc.GetProgramDependenciesResponse, error) {
	logging.V(5).Infof("GetProgramDependencies: program=%s", req.Program)

	// Parse HCL files to extract provider dependencies
	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Program)
	if diags.HasErrors() {
		return &pulumirpc.GetProgramDependenciesResponse{
			Dependencies: []*pulumirpc.DependencyInfo{},
		}, nil
	}

	var deps []*pulumirpc.DependencyInfo

	// Extract dependencies from terraform block
	if config.Terraform != nil {
		for name, provider := range config.Terraform.RequiredProviders {
			dep := &pulumirpc.DependencyInfo{
				Name: name,
			}

			if provider.Version != "" {
				dep.Version = provider.Version
			}

			if provider.Source != "" {
				parts := strings.Split(provider.Source, "/")
				if len(parts) >= 2 {
					dep.Name = parts[len(parts)-1]
				}
			}

			deps = append(deps, dep)
		}
	}

	return &pulumirpc.GetProgramDependenciesResponse{
		Dependencies: deps,
	}, nil
}

// RunPlugin runs a plugin program (for component providers).
// This allows HCL modules to be consumed as component resources from other languages.
func (host *LanguageHost) RunPlugin(
	req *pulumirpc.RunPluginRequest,
	server pulumirpc.LanguageRuntime_RunPluginServer,
) error {
	logging.V(5).Infof("RunPlugin: pwd=%s args=%v", req.Pwd, req.Args)

	// Get the module path from the request
	modulePath := req.Pwd
	if req.Info != nil && req.Info.EntryPoint != "" {
		modulePath = req.Info.EntryPoint
	}

	// Extract provider name and version from args
	name := "hcl-component"
	version := version.Version
	for i, arg := range req.Args {
		if arg == "--name" && i+1 < len(req.Args) {
			name = req.Args[i+1]
		}
		if arg == "--version" && i+1 < len(req.Args) {
			version = req.Args[i+1]
		}
	}

	// Create the provider
	provider, err := NewHCLProvider(modulePath, name, version, req.LoaderTarget)
	if err != nil {
		errBytes := []byte(fmt.Sprintf("Error creating provider: %v\n", err))
		server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
		})
		server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 1},
		})
		return nil
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	pulumirpc.RegisterResourceProviderServer(grpcServer, provider)

	// Listen on a random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		errBytes := []byte(fmt.Sprintf("Error listening: %v\n", err))
		server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
		})
		server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 1},
		})
		return nil
	}

	// Output the port for the engine to connect
	port := lis.Addr().(*net.TCPAddr).Port
	portMsg := fmt.Sprintf("%d\n", port)
	server.Send(&pulumirpc.RunPluginResponse{
		Output: &pulumirpc.RunPluginResponse_Stdout{Stdout: []byte(portMsg)},
	})

	// Start serving in a goroutine
	errCh := make(chan error, 1)
	go func() {
		errCh <- grpcServer.Serve(lis)
	}()

	// Wait for context cancellation or server error
	ctx := server.Context()
	select {
	case <-ctx.Done():
		grpcServer.GracefulStop()
	case err := <-errCh:
		if err != nil {
			errBytes := []byte(fmt.Sprintf("Server error: %v\n", err))
			server.Send(&pulumirpc.RunPluginResponse{
				Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
			})
		}
	}

	server.Send(&pulumirpc.RunPluginResponse{
		Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 0},
	})

	return nil
}

// GenerateProgram generates an HCL program from a PCL program.
func (host *LanguageHost) GenerateProgram(
	ctx context.Context,
	req *pulumirpc.GenerateProgramRequest,
) (*pulumirpc.GenerateProgramResponse, error) {
	// TODO: Implement PCL -> HCL conversion
	return nil, status.Errorf(codes.Unimplemented, "GenerateProgram not yet implemented")
}

// GenerateProject generates a complete HCL project.
func (host *LanguageHost) GenerateProject(
	ctx context.Context,
	req *pulumirpc.GenerateProjectRequest,
) (*pulumirpc.GenerateProjectResponse, error) {
	// TODO: Implement project generation
	return nil, status.Errorf(codes.Unimplemented, "GenerateProject not yet implemented")
}

// GeneratePackage generates SDK bindings for a Pulumi package.
func (host *LanguageHost) GeneratePackage(
	ctx context.Context,
	req *pulumirpc.GeneratePackageRequest,
) (*pulumirpc.GeneratePackageResponse, error) {
	// HCL doesn't need generated SDKs - it uses provider schemas directly
	return &pulumirpc.GeneratePackageResponse{}, nil
}

// Pack packages an HCL program into a deployable artifact.
func (host *LanguageHost) Pack(
	ctx context.Context,
	req *pulumirpc.PackRequest,
) (*pulumirpc.PackResponse, error) {
	// TODO: Implement packaging if needed
	return nil, status.Errorf(codes.Unimplemented, "Pack not yet implemented")
}

// Ensure LanguageHost implements the interface.
var _ pulumirpc.LanguageRuntimeServer = (*LanguageHost)(nil)

// extractSemverFromConstraint extracts a semver version from a Terraform version constraint.
// For example, ">= 4.0.0" returns "4.0.0", "~> 6.0" returns "6.0.0".
// If the constraint cannot be parsed, returns empty string (let Pulumi resolve).
func extractSemverFromConstraint(constraint string) string {
	// Remove common constraint operators
	constraint = strings.TrimSpace(constraint)
	constraint = strings.TrimPrefix(constraint, ">=")
	constraint = strings.TrimPrefix(constraint, "~>")
	constraint = strings.TrimPrefix(constraint, ">")
	constraint = strings.TrimPrefix(constraint, "=")
	constraint = strings.TrimPrefix(constraint, "^")
	constraint = strings.TrimSpace(constraint)

	// Handle multiple constraints (comma-separated) - take the first one
	if idx := strings.Index(constraint, ","); idx >= 0 {
		constraint = strings.TrimSpace(constraint[:idx])
	}

	// Validate it looks like a semver (digits and dots)
	if constraint == "" {
		return ""
	}

	// Ensure it has at least major.minor.patch format
	parts := strings.Split(constraint, ".")
	switch len(parts) {
	case 1:
		return parts[0] + ".0.0"
	case 2:
		return constraint + ".0"
	default:
		return constraint
	}
}

// resourceMonitorAdapter adapts the Pulumi gRPC resource monitor to our interface.
type resourceMonitorAdapter struct {
	client pulumirpc.ResourceMonitorClient
	ctx    context.Context
}

// RegisterResource registers a resource with Pulumi.
func (r *resourceMonitorAdapter) RegisterResource(
	ctx context.Context,
	req run.RegisterResourceRequest,
) (*run.RegisterResourceResponse, error) {
	// Convert inputs to protobuf struct
	inputsStruct, err := plugin.MarshalProperties(req.Inputs, plugin.MarshalOptions{
		KeepUnknowns: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}

	// Convert aliases from []string to []*pulumirpc.Alias
	var aliases []*pulumirpc.Alias
	for _, a := range req.Aliases {
		aliases = append(aliases, &pulumirpc.Alias{
			Alias: &pulumirpc.Alias_Urn{Urn: a},
		})
	}

	// Determine if this is a custom (provider-backed) resource
	// Stack and component resources are not custom; provider resources are
	isCustom := req.Type != "pulumi:pulumi:Stack" && !strings.HasPrefix(req.Type, "pulumi:providers:")

	// Build the registration request
	registerReq := &pulumirpc.RegisterResourceRequest{
		Type:                       req.Type,
		Name:                       req.Name,
		Custom:                     isCustom,
		Object:                     inputsStruct,
		Protect:                    &req.Protect,
		Dependencies:               req.Dependencies,
		Provider:                   req.Provider,
		Parent:                     req.Parent,
		IgnoreChanges:              req.IgnoreChanges,
		Aliases:                    aliases,
		AcceptSecrets:              true,
		AcceptResources:            true,
		SupportsPartialValues:      true,
		DeleteBeforeReplace:        req.DeleteBeforeReplace,
		DeleteBeforeReplaceDefined: req.DeleteBeforeReplaceDef,
		ImportId:                   req.ImportId,
	}

	// Add custom timeouts if specified
	if req.CustomTimeouts != nil {
		registerReq.CustomTimeouts = &pulumirpc.RegisterResourceRequest_CustomTimeouts{
			Create: formatTimeoutSeconds(req.CustomTimeouts.Create),
			Update: formatTimeoutSeconds(req.CustomTimeouts.Update),
			Delete: formatTimeoutSeconds(req.CustomTimeouts.Delete),
		}
	}

	// Call the resource monitor
	resp, err := r.client.RegisterResource(ctx, registerReq)
	if err != nil {
		return nil, fmt.Errorf("registering resource: %w", err)
	}

	// Unmarshal outputs
	outputs, err := plugin.UnmarshalProperties(resp.Object, plugin.MarshalOptions{
		KeepUnknowns: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("unmarshaling outputs: %w", err)
	}

	return &run.RegisterResourceResponse{
		URN:     resp.Urn,
		ID:      resp.Id,
		Outputs: outputs,
	}, nil
}

// Invoke invokes a provider function.
func (r *resourceMonitorAdapter) Invoke(
	ctx context.Context,
	req run.InvokeRequest,
) (*run.InvokeResponse, error) {
	// Convert args to protobuf struct
	argsStruct, err := plugin.MarshalProperties(req.Args, plugin.MarshalOptions{
		KeepUnknowns: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	// Build the invoke request
	invokeReq := &pulumirpc.ResourceInvokeRequest{
		Tok:             req.Token,
		Args:            argsStruct,
		AcceptResources: true,
	}

	// Call the resource monitor
	resp, err := r.client.Invoke(ctx, invokeReq)
	if err != nil {
		return nil, fmt.Errorf("invoking function: %w", err)
	}

	// Unmarshal return value
	returnVal, err := plugin.UnmarshalProperties(resp.Return, plugin.MarshalOptions{
		KeepUnknowns: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return value: %w", err)
	}

	// Convert failures
	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, fmt.Sprintf("%s: %s", f.Property, f.Reason))
	}

	return &run.InvokeResponse{
		Return:   returnVal,
		Failures: failures,
	}, nil
}

// RegisterResourceOutputs registers outputs on a resource (used for stack outputs).
func (r *resourceMonitorAdapter) RegisterResourceOutputs(
	ctx context.Context,
	urn string,
	outputs resource.PropertyMap,
) error {
	// Convert outputs to protobuf struct
	outputsStruct, err := plugin.MarshalProperties(outputs, plugin.MarshalOptions{
		KeepUnknowns: true,
		KeepSecrets:  true,
	})
	if err != nil {
		return fmt.Errorf("marshaling outputs: %w", err)
	}

	// Call the resource monitor
	_, err = r.client.RegisterResourceOutputs(ctx, &pulumirpc.RegisterResourceOutputsRequest{
		Urn:     urn,
		Outputs: outputsStruct,
	})
	if err != nil {
		return fmt.Errorf("registering resource outputs: %w", err)
	}

	return nil
}

// Ensure resourceMonitorAdapter implements the interface.
var _ run.ResourceMonitor = (*resourceMonitorAdapter)(nil)

// formatTimeoutSeconds converts a timeout in seconds to a duration string.
// Returns empty string if seconds is 0.
func formatTimeoutSeconds(seconds float64) string {
	if seconds == 0 {
		return ""
	}
	return time.Duration(seconds * float64(time.Second)).String()
}
