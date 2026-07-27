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
	"path/filepath"

	"github.com/blang/semver"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/schema"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
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

	// providerInfoSource resolves bridge mappings for the TF providers a
	// component uses internally. The gRPC target that serves the schema loader
	// also serves the mapper, so it is built from the same address.
	providerInfoSource bridge.ProviderInfoSource

	// packages maps a parameterized package alias to its descriptor, read from
	// the module's own sdks folder.
	packages map[string]workspace.PackageDescriptor

	// host is the host callback client.
	host pulumirpc.EngineClient

	// version is the provider version.
	version semver.Version

	// schema is the generated schema for the module.
	schema *schema.ModuleSchema

	hooks lazyCallbackServer

	// dispatchers holds each deployment's destroy-provisioner dispatcher,
	// shared by that deployment's Constructs.
	dispatchers dispatcherSet
}

// NewHCLProvider creates a new HCL component provider.
func NewHCLProvider(ctx context.Context, modulePath, addr string) (*HCLProvider, error) {
	loader := modules.NewLoader(modules.LiveResolver(ctx))
	pkgLoader, err := pulumiSchema.NewLoaderClient(addr)
	if err != nil {
		return nil, fmt.Errorf("unable to acquire schema loader: %w", err)
	}

	// The schema-loader target also serves the mapper service, so a component
	// can resolve the bridge mappings for the TF providers it uses internally.
	mapperClient, err := convert.NewMapperClient(addr)
	if err != nil {
		return nil, fmt.Errorf("unable to acquire mapper: %w", err)
	}
	providerInfoSource := bridge.NewCache(bridge.NewMapperSource(mapperClient))

	// A component's own bridged providers carry parameterization descriptors in
	// its sdks folder, mirroring how `Run` loads them for a root program.
	paramDescriptors, err := readParameterizationInfos(modulePath)
	if err != nil {
		return nil, fmt.Errorf("reading parameterization: %w", err)
	}
	schemaLoader := pulumiSchema.ReferenceLoader(pkgLoader)
	if len(paramDescriptors) > 0 {
		schemaLoader = packages.NewParameterizationAwareLoader(pkgLoader, paramDescriptors)
	}

	// Load the module to generate schema
	loaded, err := loader.LoadModule(ctx, modulePath, "", ".")
	if err != nil {
		return nil, fmt.Errorf("loading module: %w", err)
	}

	token, version, err := moduleIdentity(loaded, filepath.Base(modulePath))
	if err != nil {
		return nil, err
	}

	// Resolve resource/data source references in outputs against provider
	// schemas, using the same loader, bridge source, and required-provider hints
	// the engine uses, so schema generation types them the way Run resolves them.
	// The same module loader types references to child modules.
	cachedLoader := pulumiSchema.NewCachedLoader(schemaLoader)
	binder := &schema.Binder{
		Resources: packages.NewResolver(cachedLoader, providerInfoSource, paramDescriptors, knownProviderNames(loaded.Config)),
		Modules:   moduleLoaderAdapter{loader},
		ModuleDir: loaded.SourcePath,
	}

	moduleSchema, err := schema.GenerateModuleSchema(ctx, loaded.Config, binder, token, version)
	if err != nil {
		return nil, fmt.Errorf("generating schema: %w", err)
	}

	return &HCLProvider{
		modulePath:         modulePath,
		moduleLoader:       loader,
		pkgLoader:          cachedLoader,
		providerInfoSource: providerInfoSource,
		packages:           paramDescriptors,
		version:            version,
		schema:             moduleSchema,
	}, nil
}

// moduleIdentity derives a module's component token and version. The terraform
// `package` and `component` blocks take precedence; absent them defaultName
// supplies the package name.
//
// The module segment defaults to "index" and the component name defaults to
// "Module", so a module with no component block yields the token
// "<package>:index:Module", referenced in HCL as `<package>_module` (mirroring
// the dynamic hcl:index:Module resource).
func moduleIdentity(loaded *modules.LoadedModule, defaultName string) (tokens.Type, semver.Version, error) {
	module := "index"
	pkgName := defaultName
	pkgVersion := "0.0.0-dev"
	var explicitComponentName string
	if tf := loaded.Config.Terraform; tf != nil {
		if comp := tf.Component; comp != nil {
			explicitComponentName = comp.Name
			if comp.Module != "" {
				module = comp.Module
			}
		}
		if pkg := tf.Package; pkg != nil {
			if pkg.Name != "" {
				pkgName = pkg.Name
			}
			if pkg.Version != "" {
				pkgVersion = pkg.Version
			}
		}
	}
	componentName := "Module"
	if explicitComponentName != "" {
		componentName = explicitComponentName
	}
	version, err := semver.ParseTolerant(pkgVersion)
	if err != nil {
		return "", version, fmt.Errorf("parsing module version %q: %w", pkgVersion, err)
	}
	return tokens.Type(fmt.Sprintf("%s:%s:%s", pkgName, module, componentName)), version, nil
}

// moduleLoaderAdapter adapts *modules.Loader to schema.ModuleLoader, so schema
// generation can recursively type references to child modules.
type moduleLoaderAdapter struct {
	loader *modules.Loader
}

func (a moduleLoaderAdapter) LoadModule(
	ctx context.Context, source, versionConstraint, callerDir string,
) (*ast.Config, string, error) {
	// schema.ModuleLoader carries no context, so there is none to thread here.
	m, err := a.loader.LoadModule(ctx, source, versionConstraint, callerDir)
	if err != nil {
		return nil, "", err
	}
	return m.Config, m.SourcePath, nil
}

// Attach configures the provider with a host callback.
func (p *HCLProvider) Attach(ctx context.Context, req *pulumirpc.PluginAttach) (*emptypb.Empty, error) {
	conn, err := grpc.NewClient(
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
	schemaJSON, err := json.Marshal(p.schema.ToPulumiPackageSchema())
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
	monitorConn, err := grpc.NewClient(
		req.MonitorEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to monitor: %w", err)
	}
	defer contract.IgnoreClose(monitorConn)

	monitor := pulumirpc.NewResourceMonitorClient(monitorConn)

	// Load the module
	loaded, err := p.moduleLoader.LoadModule(ctx, p.modulePath, "", ".")
	if err != nil {
		return nil, fmt.Errorf("loading module: %w", err)
	}

	// Convert inputs from protobuf to PropertyMap
	inputs := resource.PropertyMap{}
	if req.Inputs != nil {
		inputs, err = plugin.UnmarshalProperties(req.Inputs, constructMarshalOptions())
		if err != nil {
			return nil, fmt.Errorf("unmarshaling inputs: %w", err)
		}
	}

	// Create resource monitor adapter
	resmon := &constructResourceMonitor{
		client:                  monitor,
		engine:                  p.host,
		ctx:                     ctx,
		parentURN:               req.Parent,
		componentType:           req.Type,
		componentName:           req.Name,
		componentInputs:         req.Inputs,
		aliases:                 req.Aliases,
		protect:                 req.Protect,
		dependencies:            req.Dependencies,
		providers:               req.Providers,
		ignoreChanges:           req.IgnoreChanges,
		additionalSecretOutputs: req.AdditionalSecretOutputs,
		deleteBeforeReplace:     req.DeleteBeforeReplace,
		deletedWith:             req.DeletedWith,
		retainOnDelete:          req.RetainOnDelete,
		replaceOnChanges:        req.ReplaceOnChanges,
		replaceWith:             req.ReplaceWith,
		customTimeouts:          req.CustomTimeouts,
		replacementTrigger:      req.ReplacementTrigger,
		mapOutputs:              p.schema.OutputsToPulumi,
	}
	if resmon.hooks, err = p.hooks.get(); err != nil {
		return nil, fmt.Errorf("starting hook callback server: %w", err)
	}

	// Set up config from inputs, prefixing with project name as the engine
	// expects. Inputs arrive under their camelCase schema property names (with
	// camelCase object fields); map them to the snake_case names the HCL module
	// declares, at every nesting depth. Values are passed through as already-typed
	// cty values, preserving structure, unknowns, and marks (e.g. secrets).
	config := make(map[string]run.ConfigValue)
	for k, v := range p.schema.InputsToHCL(resource.FromResourcePropertyMap(inputs)).All {
		config[req.Project+":"+string(k)] = run.TypedConfigValue(transform.PropertyValueToCty(v))
	}

	// Create and run the engine
	engine, err := run.NewEngine(ctx, loaded.Config, &run.EngineOptions{
		ProjectName:        req.Project,
		StackName:          req.Stack,
		Organization:       req.Organization,
		DryRun:             req.DryRun,
		DestroyDispatcher:  p.dispatchers.get(req.MonitorEndpoint),
		WorkDir:            loaded.SourcePath,
		RootDir:            loaded.SourcePath,
		AbsolutePaths:      true,
		Config:             config,
		ResourceMonitor:    resmon,
		SchemaLoader:       p.pkgLoader,
		ModuleLoader:       modules.NewLoader(modules.LiveResolver(ctx)),
		ProviderInfoSource: p.providerInfoSource,
		Packages:           p.packages,
	})
	if err != nil {
		return nil, fmt.Errorf("creating engine: %w", err)
	}

	if err := engine.Run(ctx); err != nil {
		return nil, fmt.Errorf("executing module: %w", err)
	}

	// Get the component URN (registered by the engine)
	componentURN := resmon.componentURN

	// Collect outputs from the resource monitor
	outputsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(resmon.outputs), constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling outputs: %w", err)
	}

	return &pulumirpc.ConstructResponse{
		Urn:               string(componentURN),
		State:             outputsStruct,
		StateDependencies: buildStateDependencies(outputsStruct),
	}, nil
}

// GetPluginInfo returns plugin metadata.
func (p *HCLProvider) GetPluginInfo(ctx context.Context, req *emptypb.Empty) (*pulumirpc.PluginInfo, error) {
	return &pulumirpc.PluginInfo{
		Version: p.version.String(),
	}, nil
}

// Cancel cancels any in-flight operations.
func (p *HCLProvider) Cancel(ctx context.Context, req *emptypb.Empty) (*emptypb.Empty, error) {
	p.hooks.close()
	return &emptypb.Empty{}, nil
}

// GetMapping returns provider mappings.
func (p *HCLProvider) GetMapping(ctx context.Context, req *pulumirpc.GetMappingRequest) (*pulumirpc.GetMappingResponse, error) {
	return &pulumirpc.GetMappingResponse{}, nil
}

// constructResourceMonitor wraps the resource monitor for Construct calls.
// It intercepts the engine's stack registration and converts it into the
// actual component resource registration expected by the Pulumi engine.
type constructResourceMonitor struct {
	client          pulumirpc.ResourceMonitorClient
	engine          pulumirpc.EngineClient
	ctx             context.Context
	parentURN       string
	componentType   string
	componentName   string
	componentInputs *structpb.Struct
	// Resource options from the ConstructRequest that must be forwarded
	// when registering the component resource with the engine.
	aliases                 []*pulumirpc.Alias
	protect                 *bool
	dependencies            []string
	providers               map[string]string
	ignoreChanges           []string
	additionalSecretOutputs []string
	deleteBeforeReplace     *bool
	deletedWith             string
	retainOnDelete          *bool
	replaceOnChanges        []string
	replaceWith             []string
	customTimeouts          *pulumirpc.ConstructRequest_CustomTimeouts
	replacementTrigger      *structpb.Value

	componentURN urn.URN
	outputs      property.Map

	// mapOutputs transforms the HCL module's top-level outputs into the
	// component's output property map before they are registered.
	mapOutputs func(property.Map) property.Map

	// hooks is the provider-owned callback server (not this monitor's): a
	// `before_delete` hook fires after this Construct call has returned, so the
	// callback server must outlive any single construct.
	hooks *callbackServer
}

// constructMarshalOptions are the property (un)marshal options used on every
// hop across the Construct boundary.
func constructMarshalOptions() plugin.MarshalOptions {
	return plugin.MarshalOptions{
		KeepUnknowns:     true,
		KeepSecrets:      true,
		KeepResources:    true,
		KeepOutputValues: true,
	}
}

// RegisterResource registers a resource.
func (m *constructResourceMonitor) RegisterResource(
	ctx context.Context,
	req run.RegisterResourceRequest,
) (*run.RegisterResourceResponse, error) {
	// Intercept the engine's internal stack registration and convert it to
	// the component resource that the Construct caller expects.
	if req.Type == "pulumi:pulumi:Stack" {
		registerReq := &pulumirpc.RegisterResourceRequest{
			Type:                    m.componentType,
			Name:                    m.componentName,
			Parent:                  m.parentURN,
			Object:                  m.componentInputs,
			Aliases:                 m.aliases,
			Protect:                 m.protect,
			Dependencies:            m.dependencies,
			Providers:               m.providers,
			IgnoreChanges:           m.ignoreChanges,
			AdditionalSecretOutputs: m.additionalSecretOutputs,
			DeletedWith:             m.deletedWith,
			RetainOnDelete:          m.retainOnDelete,
			ReplaceOnChanges:        m.replaceOnChanges,
			ReplaceWith:             m.replaceWith,
			AcceptSecrets:           true,
			AcceptResources:         true,
		}
		if m.deleteBeforeReplace != nil {
			registerReq.DeleteBeforeReplace = *m.deleteBeforeReplace
			registerReq.DeleteBeforeReplaceDefined = true
		}
		if m.customTimeouts != nil {
			registerReq.CustomTimeouts = &pulumirpc.RegisterResourceRequest_CustomTimeouts{
				Create: m.customTimeouts.Create,
				Read:   m.customTimeouts.Read,
				Update: m.customTimeouts.Update,
				Delete: m.customTimeouts.Delete,
			}
		}
		if m.replacementTrigger != nil {
			registerReq.ReplacementTrigger = m.replacementTrigger
		}
		resp, err := m.client.RegisterResource(ctx, registerReq)
		if err != nil {
			return nil, err
		}
		m.componentURN = urn.URN(resp.Urn)
		return &run.RegisterResourceResponse{
			URN: urn.URN(resp.Urn),
			ID:  resp.Id,
		}, nil
	}

	// Convert PropertyMap to protobuf
	inputs, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Inputs), constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}

	// Use parent from request or fall back to component URN
	parent := req.Parent
	if parent == "" {
		parent = m.componentURN
	}

	// Prefix child resource names with the component name to ensure URN uniqueness
	// across multiple instances of the same component type. This mirrors what other
	// Pulumi SDKs do (e.g. NodeJS uses `${name}-child`).
	name := m.componentName + "-" + req.Name

	ignoreChanges, err := globsToPropertyPaths(req.IgnoreChanges)
	if err != nil {
		return nil, fmt.Errorf("marshaling ignoreChanges: %w", err)
	}

	resp, err := m.client.RegisterResource(ctx, &pulumirpc.RegisterResourceRequest{
		Type:                req.Type,
		Name:                name,
		Custom:              req.Custom,
		Object:              inputs,
		Parent:              string(parent),
		Dependencies:        req.Dependencies,
		Provider:            req.Provider,
		Protect:             &req.Protect,
		DeleteBeforeReplace: req.DeleteBeforeReplace,
		IgnoreChanges:       ignoreChanges,
		PackageRef:          string(req.PackageRef),
		Version:             req.Version,
		PluginDownloadURL:   req.PluginDownloadURL,
		Hooks:               hooksToProto(req.Hooks),
		AcceptSecrets:       true,
		AcceptResources:     true,
	})
	if err != nil {
		return nil, err
	}

	// Convert outputs back to PropertyMap
	outputs, err := plugin.UnmarshalProperties(resp.Object, constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling outputs: %w", err)
	}

	return &run.RegisterResourceResponse{
		URN:     urn.URN(resp.Urn),
		ID:      resp.Id,
		Outputs: resource.FromResourcePropertyMap(outputs),
	}, nil
}

// ReadResource reads the state of an existing resource.
func (m *constructResourceMonitor) ReadResource(
	ctx context.Context,
	req run.ReadResourceRequest,
) (*run.ReadResourceResponse, error) {
	properties, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Inputs), plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}

	parent := req.Parent
	if parent == "" {
		parent = m.componentURN
	}

	resp, err := m.client.ReadResource(ctx, &pulumirpc.ReadResourceRequest{
		Id:                      req.ID,
		Type:                    req.Type,
		Name:                    m.componentName + "-" + req.Name,
		Parent:                  string(parent),
		Properties:              properties,
		Dependencies:            req.Dependencies,
		Provider:                req.Provider,
		Version:                 req.Version,
		AdditionalSecretOutputs: req.AdditionalSecretOutputs,
		PluginDownloadURL:       req.PluginDownloadURL,
		PackageRef:              string(req.PackageRef),
		AcceptSecrets:           true,
		AcceptResources:         true,
	})
	if err != nil {
		return nil, err
	}

	outputs, err := plugin.UnmarshalProperties(resp.Properties, plugin.MarshalOptions{
		KeepSecrets:   true,
		KeepResources: true,
	})
	if err != nil {
		return nil, fmt.Errorf("unmarshaling outputs: %w", err)
	}

	return &run.ReadResourceResponse{
		URN:     urn.URN(resp.Urn),
		ID:      req.ID,
		Outputs: resource.FromResourcePropertyMap(outputs),
	}, nil
}

// RegisterResourceOutputs registers resource outputs.
func (m *constructResourceMonitor) RegisterResourceOutputs(
	ctx context.Context,
	urn urn.URN,
	outputs property.Map,
) error {
	// The component's outputs are keyed by the HCL module's snake_case output
	// names (with snake_case object fields); expose them under the camelCase
	// names the schema declares, at every nesting depth. Child resources already
	// carry their own correctly-cased property names.
	if urn == m.componentURN {
		outputs = m.mapOutputs(outputs)
		m.outputs = outputs
	}

	outputsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(outputs), constructMarshalOptions())
	if err != nil {
		return fmt.Errorf("marshaling outputs: %w", err)
	}

	_, err = m.client.RegisterResourceOutputs(ctx, &pulumirpc.RegisterResourceOutputsRequest{
		Urn:     string(urn),
		Outputs: outputsStruct,
	})
	return err
}

// Invoke invokes a provider function.
func (m *constructResourceMonitor) Invoke(
	ctx context.Context,
	req run.InvokeRequest,
) (*run.InvokeResponse, error) {
	argsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Args), constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	resp, err := m.client.Invoke(ctx, &pulumirpc.ResourceInvokeRequest{
		Tok:               req.Token,
		Args:              argsStruct,
		Provider:          req.Provider,
		Version:           req.Version,
		PluginDownloadURL: req.PluginDownloadURL,
		PackageRef:        string(req.PackageRef),
		AcceptResources:   true,
	})
	if err != nil {
		return nil, err
	}

	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, f.Reason)
	}

	ret, err := plugin.UnmarshalProperties(resp.Return, constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return: %w", err)
	}

	return &run.InvokeResponse{
		Return:   resource.FromResourcePropertyMap(ret),
		Failures: failures,
	}, nil
}

// Call invokes a method on a resource.
func (m *constructResourceMonitor) Call(
	ctx context.Context,
	req run.CallRequest,
) (*run.CallResponse, error) {
	argsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Args), constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	resp, err := m.client.Call(ctx, &pulumirpc.ResourceCallRequest{
		Tok:        req.Token,
		Args:       argsStruct,
		PackageRef: string(req.PackageRef),
	})
	if err != nil {
		return nil, fmt.Errorf("calling method: %w", err)
	}

	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, f.Reason)
	}

	ret, err := plugin.UnmarshalProperties(resp.Return, constructMarshalOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return: %w", err)
	}

	return &run.CallResponse{
		Return:   resource.FromResourcePropertyMap(ret),
		Failures: failures,
	}, nil
}

// CheckPulumiVersion checks if the Pulumi CLI version satisfies the given version range.
func (m *constructResourceMonitor) CheckPulumiVersion(ctx context.Context, versionRange string) error {
	return nil
}

func (m *constructResourceMonitor) RegisterPackage(
	ctx context.Context,
	pkg workspace.PackageDescriptor,
) (run.PackageRef, error) {
	return registerPackage(ctx, m.client, pkg)
}

// RegisterResourceHook hosts the callback on the provider-owned callback server
// and registers it with the engine's monitor.
func (m *constructResourceMonitor) RegisterResourceHook(
	ctx context.Context, name string, callback run.ResourceHookFunction, opts run.ResourceHookOptions,
) error {
	if m.hooks == nil {
		return fmt.Errorf("resource hooks are not supported: no callback server is available")
	}
	cb, err := m.hooks.register(resourceHookCallback(callback))
	if err != nil {
		return fmt.Errorf("registering hook callback: %w", err)
	}
	_, err = m.client.RegisterResourceHook(ctx, &pulumirpc.RegisterResourceHookRequest{
		Name:     name,
		Callback: cb,
		OnDryRun: opts.OnDryRun,
	})
	if err != nil {
		return fmt.Errorf("registering hook %q: %w", name, err)
	}
	return nil
}

// LogWarning emits a non-fatal warning diagnostic to the engine.
func (m *constructResourceMonitor) LogWarning(ctx context.Context, message string) error {
	if m.engine == nil {
		return nil
	}
	_, err := m.engine.Log(ctx, &pulumirpc.LogRequest{
		Severity: pulumirpc.LogSeverity_WARNING,
		Message:  message,
	})
	return err
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

// ResolveURN mirrors RegisterResource's rewrites (parent fallback, name
// prefix), then replicates the engine's URN generation. The component URN is
// set before any resource the engine registers.
func (m *constructResourceMonitor) ResolveURN(parent urn.URN, token, name string) (urn.URN, string) {
	if parent == "" {
		parent = m.componentURN
	}
	name = m.componentName + "-" + name
	parentType := tokens.Type("")
	if parent != "" && parent.QualifiedType() != resource.RootStackType {
		parentType = parent.QualifiedType()
	}
	return urn.New(m.componentURN.Stack(), m.componentURN.Project(), parentType, tokens.Type(token), name), name
}
