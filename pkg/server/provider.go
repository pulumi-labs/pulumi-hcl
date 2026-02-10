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
	"encoding/json"
	"fmt"

	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/modules"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/schema"
	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// HCLProvider implements a Pulumi provider that serves HCL modules as components.
type HCLProvider struct {
	pulumirpc.UnimplementedResourceProviderServer

	// modulePath is the path to the HCL module directory.
	modulePath string

	// moduleLoader loads HCL modules.
	moduleLoader *modules.Loader

	// pkgLoader loads provider schemas.
	pkgLoader pulumiSchema.ReferenceLoader

	// host is the host callback client.
	host pulumirpc.EngineClient

	// name is the provider name.
	name string

	// version is the provider version.
	version string

	// schema is the generated schema for the module.
	schema *schema.ModuleSchema
}

// NewHCLProvider creates a new HCL component provider.
func NewHCLProvider(modulePath, name, version, addr string) (*HCLProvider, error) {
	loader := modules.NewLoader()
	pkgLoader, err := pulumiSchema.NewLoaderClient(addr)
	if err != nil {
		return nil, fmt.Errorf("unable to acquire schema loader: %w", err)
	}

	// Load the module to generate schema
	loaded, err := loader.LoadModule(modulePath, ".")
	if err != nil {
		return nil, fmt.Errorf("loading module: %w", err)
	}

	// Generate schema from the module
	moduleSchema, err := schema.GenerateModuleSchema(loaded.Config, name, version)
	if err != nil {
		return nil, fmt.Errorf("generating schema: %w", err)
	}

	return &HCLProvider{
		modulePath:   modulePath,
		moduleLoader: loader,
		pkgLoader:    pulumiSchema.NewCachedLoader(pkgLoader),
		name:         name,
		version:      version,
		schema:       moduleSchema,
	}, nil
}

// Attach configures the provider with a host callback.
func (p *HCLProvider) Attach(ctx context.Context, req *pulumirpc.PluginAttach) (*emptypb.Empty, error) {
	conn, err := grpc.Dial(
		req.Address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to host: %w", err)
	}
	p.host = pulumirpc.NewEngineClient(conn)
	return &emptypb.Empty{}, nil
}

// GetSchema returns the schema for the HCL module.
func (p *HCLProvider) GetSchema(ctx context.Context, req *pulumirpc.GetSchemaRequest) (*pulumirpc.GetSchemaResponse, error) {
	schemaJSON, err := json.Marshal(p.schema.ToPulumiPackageSchema(p.name))
	if err != nil {
		return nil, fmt.Errorf("marshaling schema: %w", err)
	}
	return &pulumirpc.GetSchemaResponse{
		Schema: string(schemaJSON),
	}, nil
}

// CheckConfig validates provider configuration.
func (p *HCLProvider) CheckConfig(ctx context.Context, req *pulumirpc.CheckRequest) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{Inputs: req.News}, nil
}

// DiffConfig computes configuration differences.
func (p *HCLProvider) DiffConfig(ctx context.Context, req *pulumirpc.DiffRequest) (*pulumirpc.DiffResponse, error) {
	return &pulumirpc.DiffResponse{}, nil
}

// Configure configures the provider.
func (p *HCLProvider) Configure(ctx context.Context, req *pulumirpc.ConfigureRequest) (*pulumirpc.ConfigureResponse, error) {
	return &pulumirpc.ConfigureResponse{
		AcceptSecrets:   true,
		SupportsPreview: true,
	}, nil
}

// Check validates resource inputs.
func (p *HCLProvider) Check(ctx context.Context, req *pulumirpc.CheckRequest) (*pulumirpc.CheckResponse, error) {
	return &pulumirpc.CheckResponse{Inputs: req.News}, nil
}

// Diff computes resource differences.
func (p *HCLProvider) Diff(ctx context.Context, req *pulumirpc.DiffRequest) (*pulumirpc.DiffResponse, error) {
	return &pulumirpc.DiffResponse{}, nil
}

// Read reads resource state.
func (p *HCLProvider) Read(ctx context.Context, req *pulumirpc.ReadRequest) (*pulumirpc.ReadResponse, error) {
	return &pulumirpc.ReadResponse{
		Id:         req.Id,
		Properties: req.Properties,
	}, nil
}

// Delete deletes a resource.
func (p *HCLProvider) Delete(ctx context.Context, req *pulumirpc.DeleteRequest) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// Construct creates a component resource by executing the HCL module.
func (p *HCLProvider) Construct(ctx context.Context, req *pulumirpc.ConstructRequest) (*pulumirpc.ConstructResponse, error) {
	logging.V(5).Infof("Construct: type=%s name=%s", req.Type, req.Name)

	// Connect to the resource monitor
	monitorConn, err := grpc.Dial(
		req.MonitorEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to monitor: %w", err)
	}
	defer monitorConn.Close()

	monitor := pulumirpc.NewResourceMonitorClient(monitorConn)

	// Load the module
	loaded, err := p.moduleLoader.LoadModule(p.modulePath, ".")
	if err != nil {
		return nil, fmt.Errorf("loading module: %w", err)
	}

	// Convert inputs from protobuf to PropertyMap
	inputs := resource.PropertyMap{}
	if req.Inputs != nil {
		inputs, err = plugin.UnmarshalProperties(req.Inputs, plugin.MarshalOptions{
			KeepSecrets:   true,
			KeepResources: true,
		})
		if err != nil {
			return nil, fmt.Errorf("unmarshaling inputs: %w", err)
		}
	}

	// Create resource monitor adapter
	resmon := &constructResourceMonitor{
		client:    monitor,
		ctx:       ctx,
		parentURN: req.Parent,
	}

	// Set up config from inputs
	config := make(map[string]string)
	for k, v := range inputs {
		if v.IsString() {
			config[string(k)] = v.StringValue()
		} else {
			// Convert non-string values to JSON
			jsonVal, _ := json.Marshal(v.V)
			config[string(k)] = string(jsonVal)
		}
	}

	// Create engine options
	engineOpts := &run.EngineOptions{
		ProjectName:     req.Project,
		StackName:       req.Stack,
		DryRun:          req.DryRun,
		WorkDir:         p.modulePath,
		Config:          config,
		ResourceMonitor: resmon,
	}

	// Create and run the engine
	engine := run.NewEngine(loaded.Config, engineOpts)

	if err := engine.Run(ctx); err != nil {
		return nil, fmt.Errorf("executing module: %w", err)
	}

	// Get the component URN (registered by the engine)
	componentURN := resmon.componentURN

	// Collect outputs from the resource monitor
	outputsStruct, err := plugin.MarshalProperties(resmon.outputs, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling outputs: %w", err)
	}

	return &pulumirpc.ConstructResponse{
		Urn:               componentURN,
		State:             outputsStruct,
		StateDependencies: buildStateDependencies(outputsStruct),
	}, nil
}

// GetPluginInfo returns plugin metadata.
func (p *HCLProvider) GetPluginInfo(ctx context.Context, req *emptypb.Empty) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: p.version,
	}, nil
}

// Cancel cancels any in-flight operations.
func (p *HCLProvider) Cancel(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	return &emptypb.Empty{}, nil
}

// GetMapping returns provider mappings.
func (p *HCLProvider) GetMapping(ctx context.Context, req *pulumirpc.GetMappingRequest) (*pulumirpc.GetMappingResponse, error) {
	return &pulumirpc.GetMappingResponse{}, nil
}

// constructResourceMonitor wraps the resource monitor for Construct calls.
type constructResourceMonitor struct {
	client       pulumirpc.ResourceMonitorClient
	ctx          context.Context
	parentURN    string
	componentURN string
	outputs      resource.PropertyMap
}

// RegisterResource registers a resource.
func (m *constructResourceMonitor) RegisterResource(
	ctx context.Context,
	req run.RegisterResourceRequest,
) (*run.RegisterResourceResponse, error) {
	// Convert PropertyMap to protobuf
	inputs, err := plugin.MarshalProperties(req.Inputs, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}

	// Use parent from request or fall back to construct parent
	parent := req.Parent
	if parent == "" {
		parent = m.parentURN
	}

	resp, err := m.client.RegisterResource(ctx, &pulumirpc.RegisterResourceRequest{
		Type:                req.Type,
		Name:                req.Name,
		Object:              inputs,
		Parent:              parent,
		Dependencies:        req.Dependencies,
		Protect:             &req.Protect,
		DeleteBeforeReplace: req.DeleteBeforeReplace,
		IgnoreChanges:       req.IgnoreChanges,
		AcceptSecrets:       true,
		AcceptResources:     true,
	})
	if err != nil {
		return nil, err
	}

	// Track the first component URN (the root stack)
	if m.componentURN == "" {
		m.componentURN = resp.Urn
	}

	// Convert outputs back to PropertyMap
	outputs, err := plugin.UnmarshalProperties(resp.Object, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
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

// RegisterResourceOutputs registers resource outputs.
func (m *constructResourceMonitor) RegisterResourceOutputs(
	ctx context.Context,
	urn string,
	outputs resource.PropertyMap,
) error {
	// Track outputs for the component
	if urn == m.componentURN {
		m.outputs = outputs
	}

	outputsStruct, err := plugin.MarshalProperties(outputs, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return fmt.Errorf("marshaling outputs: %w", err)
	}

	_, err = m.client.RegisterResourceOutputs(ctx, &pulumirpc.RegisterResourceOutputsRequest{
		Urn:     urn,
		Outputs: outputsStruct,
	})
	return err
}

// Invoke invokes a provider function.
func (m *constructResourceMonitor) Invoke(
	ctx context.Context,
	req run.InvokeRequest,
) (*run.InvokeResponse, error) {
	argsStruct, err := plugin.MarshalProperties(req.Args, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	resp, err := m.client.Invoke(ctx, &pulumirpc.ResourceInvokeRequest{
		Tok:             req.Token,
		Args:            argsStruct,
		AcceptResources: true,
	})
	if err != nil {
		return nil, err
	}

	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, f.Reason)
	}

	ret, err := plugin.UnmarshalProperties(resp.Return, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return: %w", err)
	}

	return &run.InvokeResponse{
		Return:   ret,
		Failures: failures,
	}, nil
}

// buildStateDependencies builds the state dependencies map from outputs.
func buildStateDependencies(outputs *structpb.Struct) map[string]*pulumirpc.ConstructResponse_PropertyDependencies {
	deps := make(map[string]*pulumirpc.ConstructResponse_PropertyDependencies)
	if outputs == nil {
		return deps
	}
	for k := range outputs.Fields {
		deps[k] = &pulumirpc.ConstructResponse_PropertyDependencies{
			Urns: []string{},
		}
	}
	return deps
}

// Ensure HCLProvider implements the interface.
var _ pulumirpc.ResourceProviderServer = (*HCLProvider)(nil)

// Ensure constructResourceMonitor implements run.ResourceMonitor.
var _ run.ResourceMonitor = (*constructResourceMonitor)(nil)
