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
	"time"

	p "github.com/pulumi/pulumi-go-provider"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/resolve"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/run"
)

// moduleProvider is the fully dynamic HCL provider. It serves the single
// hcl:index:Module resource: a component whose module source arrives as a plain
// input at Construct time, rather than being baked into a per-module schema.
//
// The services it needs to resolve and type a module's providers — a schema
// loader, a bridge mapper, and a package resolver — are supplied by the engine
// during Handshake, the only point at which they are available.
type moduleProvider struct {
	version      string
	moduleLoader *modules.Loader

	engine             pulumirpc.EngineClient
	schemaLoader       pulumiSchema.ReferenceLoader
	providerInfoSource bridge.ProviderInfoSource
	resolver           pulumirpc.PackageResolverClient
}

// NewModuleProvider builds the fully dynamic HCL provider on top of the raw
// (non-infer) pulumi-go-provider Provider surface.
func NewModuleProvider(ctx context.Context, version string) p.Provider {
	m := &moduleProvider{
		version:      version,
		moduleLoader: modules.NewLoader(ctx),
	}
	return p.Provider{
		Handshake: m.handshake,
		GetSchema: m.getSchema,
		Configure: func(context.Context, p.ConfigureRequest) error { return nil },
		CheckConfig: func(_ context.Context, req p.CheckRequest) (p.CheckResponse, error) {
			return p.CheckResponse{Inputs: req.Inputs}, nil
		},
		DiffConfig: func(context.Context, p.DiffRequest) (p.DiffResponse, error) {
			return p.DiffResponse{}, nil
		},
		Construct: m.construct,
	}
}

// handshake captures the schema loader, bridge mapper, and package resolver the
// engine exposes. All three are required: the dynamic Module cannot resolve or
// type a module's providers without them.
func (m *moduleProvider) handshake(ctx context.Context, req p.HandshakeRequest) (p.HandshakeResponse, error) {
	if req.LoaderAddress == nil || *req.LoaderAddress == "" {
		return p.HandshakeResponse{}, fmt.Errorf("no loader target received during handshake")
	}
	if req.MapperAddress == nil || *req.MapperAddress == "" {
		return p.HandshakeResponse{}, fmt.Errorf("no mapper target received during handshake")
	}
	if req.ResolverAddress == nil || *req.ResolverAddress == "" {
		return p.HandshakeResponse{}, fmt.Errorf("no resolver target received during handshake")
	}

	schemaLoader, err := pulumiSchema.NewLoaderClient(*req.LoaderAddress)
	if err != nil {
		return p.HandshakeResponse{}, fmt.Errorf("dial schema loader at %s: %w", *req.LoaderAddress, err)
	}

	mapperClient, err := convert.NewMapperClient(*req.MapperAddress)
	if err != nil {
		return p.HandshakeResponse{}, fmt.Errorf("dial mapper at %s: %w", *req.MapperAddress, err)
	}

	resolverConn, err := grpc.NewClient(*req.ResolverAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()))
	if err != nil {
		return p.HandshakeResponse{}, fmt.Errorf("dial resolver at %s: %w", *req.ResolverAddress, err)
	}

	m.schemaLoader = schemaLoader
	m.providerInfoSource = bridge.NewCache(bridge.NewMapperSource(mapperClient))
	m.resolver = pulumirpc.NewPackageResolverClient(resolverConn)
	if req.EngineAddress != "" && m.engine == nil {
		if engineConn, err := grpc.NewClient(req.EngineAddress,
			grpc.WithTransportCredentials(insecure.NewCredentials())); err == nil {
			m.engine = pulumirpc.NewEngineClient(engineConn)
		}
	}

	return p.HandshakeResponse{}, nil
}

// getSchema returns the static hcl:index:Module schema.
func (m *moduleProvider) getSchema(context.Context, p.GetSchemaRequest) (p.GetSchemaResponse, error) {
	b, err := json.Marshal(moduleResourceSchema(m.version))
	if err != nil {
		return p.GetSchemaResponse{}, fmt.Errorf("marshaling schema: %w", err)
	}
	return p.GetSchemaResponse{Schema: string(b)}, nil
}

// construct loads the module named by the "source" input, resolves the providers
// it references through the handshake resolver, runs it, and returns its outputs
// under the component's single "outputs" property.
func (m *moduleProvider) construct(ctx context.Context, req p.ConstructRequest) (p.ConstructResponse, error) {
	logging.V(5).Infof("Construct: type=%s name=%s", req.Urn.Type(), req.Urn.Name())

	schemaLoader, providerInfoSource, resolver, engine := m.schemaLoader, m.providerInfoSource, m.resolver, m.engine
	if resolver == nil {
		return p.ConstructResponse{}, fmt.Errorf("Construct called before a successful Handshake")
	}

	sourceVal, ok := req.Inputs.GetOk("source")
	if !ok || !sourceVal.IsString() {
		return p.ConstructResponse{}, fmt.Errorf("Module requires a plain string %q input", "source")
	}
	source := sourceVal.AsString()

	var inputs property.Map
	if v, ok := req.Inputs.GetOk("inputs"); ok && v.IsMap() {
		inputs = v.AsMap()
	}

	loaded, err := m.moduleLoader.LoadModule(source, "", ".")
	if err != nil {
		return p.ConstructResponse{}, fmt.Errorf("loading module %q: %w", source, err)
	}

	// Resolve every provider the module references to a concrete descriptor, the
	// dynamic equivalent of the on-disk sdks/ descriptors a source MLC reads.
	resolved, err := resolve.Packages(ctx, resolver, m.requirementSpecs(ctx, loaded.Config, loaded.SourcePath))
	if err != nil {
		return p.ConstructResponse{}, fmt.Errorf("resolving module providers: %w", err)
	}

	// A parameterization-aware loader lets the engine load the schemas of bridged
	// providers resolved above.
	loader := packages.NewParameterizationAwareLoader(schemaLoader, resolved)

	monitorConn, err := grpc.NewClient(req.MonitorEndpoint,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
	)
	if err != nil {
		return p.ConstructResponse{}, fmt.Errorf("connecting to monitor: %w", err)
	}
	defer contract.IgnoreClose(monitorConn)

	componentInputs, err := plugin.MarshalProperties(
		resource.ToResourcePropertyMap(req.Inputs),
		plugin.MarshalOptions{KeepSecrets: true, KeepResources: true},
	)
	if err != nil {
		return p.ConstructResponse{}, fmt.Errorf("marshaling inputs: %w", err)
	}

	resmon := &constructResourceMonitor{
		client:                  pulumirpc.NewResourceMonitorClient(monitorConn),
		engine:                  engine,
		ctx:                     ctx,
		parentURN:               string(req.Parent),
		componentType:           string(req.Urn.Type()),
		componentName:           string(req.Urn.Name()),
		componentInputs:         componentInputs,
		aliases:                 aliasURNsToProto(req.Aliases),
		protect:                 req.Protect,
		dependencies:            urnsToStrings(req.Dependencies),
		providers:               providersToProto(req.Providers),
		additionalSecretOutputs: req.AdditionalSecretOutputs,
		deletedWith:             string(req.DeletedWith),
		deleteBeforeReplace:     req.DeleteBeforeReplace,
		ignoreChanges:           req.IgnoreChanges,
		replaceOnChanges:        req.ReplaceOnChanges,
		retainOnDelete:          req.RetainOnDelete,
		customTimeouts:          customTimeoutsToProto(req.CustomTimeouts),
		mapOutputs:              wrapModuleOutputs,
	}

	engineRun := run.NewEngine(ctx, loaded.Config, &run.EngineOptions{
		ProjectName:        string(req.Urn.Project()),
		StackName:          string(req.Urn.Stack()),
		DryRun:             req.DryRun,
		WorkDir:            loaded.SourcePath,
		RootDir:            loaded.SourcePath,
		Config:             moduleConfig(string(req.Urn.Project()), inputs),
		ResourceMonitor:    resmon,
		SchemaLoader:       pulumiSchema.NewCachedLoader(loader),
		ProviderInfoSource: providerInfoSource,
		Packages:           resolved,
		Parallel:           int(req.Parallel),
	})

	if err := engineRun.Run(ctx); err != nil {
		return p.ConstructResponse{}, fmt.Errorf("executing module %q: %w", source, err)
	}

	return p.ConstructResponse{
		Urn:   resmon.componentURN,
		State: resmon.outputs,
	}, nil
}

// requirementSpecs turns every provider the module tree references into a
// resolver request keyed by its local name. Pulumi-sourced providers resolve by
// package name; bridged Terraform providers resolve as a parameterization of the
// terraform-provider plugin, matching how GetRequiredPackages emits install
// specs. Built-in providers (pulumi, terraform) are handled by the engine and
// are not resolved.
func (m *moduleProvider) requirementSpecs(
	ctx context.Context, config *ast.Config, workDir string,
) []resolve.Request {
	tf, pulumi, aliases := collectRequirements(ctx, config, workDir)
	var reqs []resolve.Request
	for _, alias := range sortedKeys(aliases) {
		if isBuiltinProvider(alias) {
			continue
		}
		req := aliases[alias]
		if req.IsPulumi() {
			name := pulumiPackageName(alias, req.Source)
			reqs = append(reqs, resolve.Request{
				Alias: alias,
				Spec:  &pulumirpc.PackageSpec{Source: name, Version: pulumi[name]},
			})
			continue
		}
		source := tfProviderSource(alias, req)
		params := []string{source}
		if vs := tf[source]; vs != nil {
			if c := vs.constraint(); c != "" {
				params = append(params, c)
			}
		}
		reqs = append(reqs, resolve.Request{
			Alias: alias,
			Spec:  &pulumirpc.PackageSpec{Source: "terraform-provider", Parameters: params},
		})
	}
	return reqs
}

// moduleConfig maps the module's input variables to the engine's config map,
// keyed <project>:<variable>. Non-string values are JSON-encoded, the form the
// HCL engine decodes config values from.
func moduleConfig(project string, inputs property.Map) map[string]string {
	config := make(map[string]string, inputs.Len())
	for k, v := range resource.ToResourcePropertyMap(inputs) {
		key := project + ":" + string(k)
		if v.IsString() {
			config[key] = v.StringValue()
			continue
		}
		jsonVal, _ := json.Marshal(v.Mappable())
		config[key] = string(jsonVal)
	}
	return config
}

// wrapModuleOutputs exposes the module's top-level outputs under the component's
// single untyped "outputs" property.
func wrapModuleOutputs(outputs property.Map) property.Map {
	return property.NewMap(map[string]property.Value{
		"outputs": property.New(outputs.AsMap()),
	})
}

func aliasURNsToProto(urns []resource.URN) []*pulumirpc.Alias {
	if len(urns) == 0 {
		return nil
	}
	out := make([]*pulumirpc.Alias, len(urns))
	for i, u := range urns {
		out[i] = &pulumirpc.Alias{Alias: &pulumirpc.Alias_Urn{Urn: string(u)}}
	}
	return out
}

func urnsToStrings(urns []resource.URN) []string {
	if len(urns) == 0 {
		return nil
	}
	out := make([]string, len(urns))
	for i, u := range urns {
		out[i] = string(u)
	}
	return out
}

func providersToProto(provs map[tokens.Package]p.ProviderReference) map[string]string {
	if len(provs) == 0 {
		return nil
	}
	out := make(map[string]string, len(provs))
	for pkg, ref := range provs {
		out[string(pkg)] = string(ref.Urn) + "::" + string(ref.ID)
	}
	return out
}

func customTimeoutsToProto(ct *resource.CustomTimeouts) *pulumirpc.ConstructRequest_CustomTimeouts {
	if ct == nil {
		return nil
	}
	dur := func(seconds float64) string {
		if seconds == 0 {
			return ""
		}
		return (time.Duration(seconds) * time.Second).String()
	}
	return &pulumirpc.ConstructRequest_CustomTimeouts{
		Create: dur(ct.Create),
		Update: dur(ct.Update),
		Delete: dur(ct.Delete),
	}
}
