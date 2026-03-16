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
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"maps"
	"net"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi-language-hcl/pkg/codegen"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/run"
	"github.com/pulumi/pulumi-language-hcl/pkg/version"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/fsutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/protobuf/types/known/emptypb"
	"gopkg.in/yaml.v3"
)

// LanguageHost implements the LanguageRuntimeServer gRPC interface.
type LanguageHost struct {
	pulumirpc.UnimplementedLanguageRuntimeServer
	engine  pulumirpc.EngineClient
	closers []io.Closer
}

// NewLanguageHost creates a new HCL language host.
//
// The returned [LanguageHost] should be closed.
func NewLanguageHost(engineAddress string) (*LanguageHost, error) {
	engineConn, err := grpc.NewClient(
		engineAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		rpcutil.GrpcChannelOptions(),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to engine: %w", err)
	}

	return &LanguageHost{
		engine:  pulumirpc.NewEngineClient(engineConn),
		closers: []io.Closer{engineConn},
	}, nil
}

func (host *LanguageHost) Close() error {
	errs := make([]error, len(host.closers))
	for i, v := range host.closers {
		errs[i] = v.Close()
	}
	return errors.Join(errs...)
}

// GetRequiredPackages returns the packages required to run an HCL program,
// including parameterization info for parameterized packages.
func (host *LanguageHost) GetRequiredPackages(
	ctx context.Context,
	req *pulumirpc.GetRequiredPackagesRequest,
) (*pulumirpc.GetRequiredPackagesResponse, error) {
	logging.V(5).Infof("GetRequiredPackages: program=%s", req.Info.ProgramDirectory)

	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Info.ProgramDirectory)
	if diags.HasErrors() {
		return &pulumirpc.GetRequiredPackagesResponse{}, nil
	}

	paramInfos, err := readParameterizationInfos(req.Info.ProgramDirectory)
	if err != nil {
		return &pulumirpc.GetRequiredPackagesResponse{}, fmt.Errorf("unable to read SDKs folder: %w", err)
	}

	var pkgs []*pulumirpc.PackageDependency
	if config.Terraform != nil {
		for alias, provider := range config.Terraform.RequiredProviders {

			version := func(v *semver.Version) string {
				if v == nil {
					return ""
				}
				return v.String()
			}

			parameterization := func(p *workspace.Parameterization) *pulumirpc.PackageParameterization {
				if p == nil {
					return nil
				}
				return &pulumirpc.PackageParameterization{
					Name:    p.Name,
					Version: p.Version.String(),
					Value:   p.Value,
				}
			}

			if info, ok := paramInfos[alias]; ok {
				pkgs = append(pkgs, &pulumirpc.PackageDependency{
					Name:             info.Name,
					Version:          version(info.Version),
					Kind:             "resource",
					Parameterization: parameterization(info.Parameterization),
				})
				continue
			}
			dep := &pulumirpc.PackageDependency{
				Name: alias,
				Kind: "resource",
			}
			if provider.Version != "" {
				dep.Version = run.ExtractSemverFromConstraint(provider.Version)
			}
			if provider.Source != "" {
				parts := strings.Split(provider.Source, "/")
				if len(parts) >= 2 {
					dep.Name = parts[len(parts)-1]
				}
			}
			pkgs = append(pkgs, dep)
		}
	}

	return &pulumirpc.GetRequiredPackagesResponse{Packages: pkgs}, nil
}

// readParameterizationInfos reads .hcl/sdks/*/hcl.sdk.json files from dir and
// returns a map from parameterized package alias to its ParameterizationInfo.
func readParameterizationInfos(dir string) (map[string]workspace.PackageDescriptor, error) {
	sdksDir := filepath.Join(dir, ".hcl", "sdks")
	entries, err := os.ReadDir(sdksDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]workspace.PackageDescriptor, len(entries))
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(sdksDir, entry.Name(), "hcl.sdk.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var desc workspace.PackageDescriptor
		if err := json.Unmarshal(data, &desc); err != nil {
			errs = append(errs, fmt.Errorf("%q: %w", path, err))
		} else {
			result[entry.Name()] = desc
		}
	}
	return result, errors.Join(errs...)
}

// Run executes an HCL program.
func (host *LanguageHost) Run(
	ctx context.Context,
	req *pulumirpc.RunRequest,
) (*pulumirpc.RunResponse, error) {
	logging.V(5).Infof("Run: program=%s, pwd=%s, stack=%s, project=%s",
		req.Info.EntryPoint, req.Pwd, req.Stack, req.Project)

	// Connect to the resource monitor
	monitorConn, err := grpc.NewClient(
		req.MonitorAddress,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		rpcutil.GrpcChannelOptions(),
	)
	if err != nil {
		return nil, fmt.Errorf("connecting to resource monitor: %w", err)
	}
	defer contract.IgnoreClose(monitorConn)

	// Create the resource monitor and engine clients
	monitorClient := pulumirpc.NewResourceMonitorClient(monitorConn)
	resmon := &resourceMonitorAdapter{
		monitorClient: monitorClient,
		engineClient:  host.engine,
		ctx:           ctx,
	}

	// Parse the HCL program
	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Info.ProgramDirectory)
	if diags.HasErrors() {
		return &pulumirpc.RunResponse{
			Error: diags.Error(),
		}, nil
	}

	configMap := maps.Clone(req.Config)

	schemaLoader, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("unable to acquire gRPC schema loader: %w", err)
	}

	descriptors, err := readParameterizationInfos(req.Info.ProgramDirectory)
	if err != nil {
		return nil, fmt.Errorf("unable to read parameterization: %w", err)
	}

	paramDescriptors := make(map[string]workspace.PackageDescriptor)
	for alias, desc := range descriptors {
		if desc.Parameterization != nil {
			paramDescriptors[alias] = desc
		}
	}

	loader := schema.ReferenceLoader(schemaLoader)
	if len(paramDescriptors) > 0 {
		loader = packages.NewParameterizationAwareLoader(schemaLoader, paramDescriptors)
	}

	req.Parallel = max(req.Parallel, 1) // (req.Parallel <= 1) => serial

	// Create and run the engine
	engine := run.NewEngine(config, &run.EngineOptions{
		ProjectName:      req.Project,
		StackName:        req.Stack,
		Organization:     req.Organization,
		Config:           configMap,
		ConfigSecretKeys: req.ConfigSecretKeys,
		DryRun:           req.DryRun,
		ResourceMonitor:  resmon,
		SchemaLoader:     schema.NewCachedLoader(loader),
		WorkDir:          req.Info.ProgramDirectory,
		RootDir:          req.Info.RootDirectory,
		Packages:         paramDescriptors,
		Parallel:         int(req.Parallel),
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
	logging.V(5).Infof("GetProgramDependencies: program=%s", req.Info.ProgramDirectory)

	// Parse HCL files to extract provider dependencies
	p := parser.NewParser()
	config, diags := p.ParseDirectory(req.Info.ProgramDirectory)
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
		errBytes := fmt.Appendf(nil, "Error creating provider: %v\n", err)
		if err := server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
		}); err != nil {
			return err
		}
		return server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 1},
		})
	}

	// Create gRPC server
	grpcServer := grpc.NewServer()
	pulumirpc.RegisterResourceProviderServer(grpcServer, provider)

	// Listen on a random port
	lis, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		errBytes := fmt.Appendf(nil, "Error listening: %v\n", err)
		if err := server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
		}); err != nil {
			return err
		}
		return server.Send(&pulumirpc.RunPluginResponse{
			Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 1},
		})
	}

	// Output the port for the engine to connect
	port := lis.Addr().(*net.TCPAddr).Port
	portMsg := fmt.Sprintf("%d\n", port)
	if err := server.Send(&pulumirpc.RunPluginResponse{
		Output: &pulumirpc.RunPluginResponse_Stdout{Stdout: []byte(portMsg)},
	}); err != nil {
		return err
	}

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
			errBytes := fmt.Appendf(nil, "Server error: %v\n", err)
			if err := server.Send(&pulumirpc.RunPluginResponse{
				Output: &pulumirpc.RunPluginResponse_Stderr{Stderr: errBytes},
			}); err != nil {
				return err
			}
		}
	}

	return server.Send(&pulumirpc.RunPluginResponse{
		Output: &pulumirpc.RunPluginResponse_Exitcode{Exitcode: 0},
	})
}

// GenerateProgram generates an HCL program from a PCL program.
func (host *LanguageHost) GenerateProgram(
	ctx context.Context,
	req *pulumirpc.GenerateProgramRequest,
) (*pulumirpc.GenerateProgramResponse, error) {
	if len(req.Source) == 0 {
		return &pulumirpc.GenerateProgramResponse{
			Source: map[string][]byte{"main.hcl": {}},
		}, nil
	}

	// Write source files to a temp directory so that BindDirectory can resolve
	// component references (which require a directory path on the filesystem).
	tmpDir, err := os.MkdirTemp("", "hcl-generate-*")
	if err != nil {
		return nil, fmt.Errorf("creating temp directory: %w", err)
	}
	defer contract.IgnoreError(os.RemoveAll(tmpDir))
	for k, v := range req.Source {
		p := filepath.Join(tmpDir, k)
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			return nil, fmt.Errorf("creating directory for %s: %w", k, err)
		}
		if err := os.WriteFile(p, []byte(v), 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", k, err)
		}
	}

	loaderClient, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("unable to aquire loader: %w", err)
	}
	binderOpts := []pcl.BindOption{pcl.Loader(schema.NewCachedLoader(loaderClient))}
	if !req.Strict {
		binderOpts = append(binderOpts, pcl.NonStrictBindOptions()...)
	}
	program, bindDiags, err := pcl.BindDirectory(tmpDir, nil, binderOpts...)
	if err != nil {
		return nil, fmt.Errorf("binding program: %w", err)
	}
	if bindDiags.HasErrors() {
		return &pulumirpc.GenerateProgramResponse{
			Diagnostics: plugin.HclDiagnosticsToRPCDiagnostics(bindDiags),
		}, nil
	}

	files, genDiags, err := codegen.GenerateProgram(program)
	if err != nil {
		return nil, fmt.Errorf("generating program: %w", err)
	}

	// Include .hcl/sdks/<alias>/hcl.sdk.json for parameterized packages so that
	// ConvertProgram can load their schemas when the round-trip test writes these
	// files to a temp directory and then calls ConvertProgram on that directory.
	for _, ref := range program.PackageReferences() {
		if ref.Name() == "pulumi" {
			continue
		}
		pkg, pkgErr := ref.Definition()
		if pkgErr != nil || pkg.Parameterization == nil {
			continue
		}
		baseVersion := pkg.Parameterization.BaseProvider.Version
		var paramVersion semver.Version
		if pkg.Version != nil {
			paramVersion = *pkg.Version
		}
		desc := workspace.PackageDescriptor{
			PluginDescriptor: workspace.PluginDescriptor{
				Name:    pkg.Parameterization.BaseProvider.Name,
				Version: &baseVersion,
				Kind:    apitype.ResourcePlugin,
			},
			Parameterization: &workspace.Parameterization{
				Name:    pkg.Name,
				Version: paramVersion,
				Value:   pkg.Parameterization.Parameter,
			},
		}
		data, marshalErr := json.Marshal(desc)
		if marshalErr != nil {
			continue
		}
		files[".hcl/sdks/"+pkg.Name+"/hcl.sdk.json"] = data
	}

	return &pulumirpc.GenerateProgramResponse{
		Diagnostics: plugin.HclDiagnosticsToRPCDiagnostics(genDiags),
		Source:      files,
	}, nil
}

// GenerateProject generates a complete HCL project.
func (host *LanguageHost) GenerateProject(
	ctx context.Context,
	req *pulumirpc.GenerateProjectRequest,
) (*pulumirpc.GenerateProjectResponse, error) {
	logging.V(5).Infof("GenerateProject: sourceDirectory=%s, targetDirectory=%s",
		req.SourceDirectory, req.TargetDirectory)

	loaderClient, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("unable to aquire loader: %w", err)
	}
	binderOpts := []pcl.BindOption{pcl.Loader(schema.NewCachedLoader(loaderClient))}
	if !req.Strict {
		binderOpts = append(binderOpts, pcl.NonStrictBindOptions()...)
	}
	program, bindDiags, err := pcl.BindDirectory(req.SourceDirectory, nil, binderOpts...)
	if err != nil {
		return nil, fmt.Errorf("binding directory: %w", err)
	}
	if bindDiags.HasErrors() {
		return &pulumirpc.GenerateProjectResponse{
			Diagnostics: plugin.HclDiagnosticsToRPCDiagnostics(bindDiags),
		}, nil
	}

	files, genDiags, err := codegen.GenerateProgram(program)
	if err != nil {
		return nil, fmt.Errorf("generating program: %w", err)
	}

	// Determine where to write program files. When the project specifies a
	// "main" subdirectory, generated code goes into that subdirectory.
	programDir := req.TargetDirectory
	var project map[string]any
	if err := json.Unmarshal([]byte(req.Project), &project); err != nil {
		return nil, fmt.Errorf("parsing project JSON: %w", err)
	}
	if main, ok := project["main"].(string); ok && main != "" {
		programDir = filepath.Join(req.TargetDirectory, main)
		if err := os.MkdirAll(programDir, 0755); err != nil {
			return nil, fmt.Errorf("creating main directory: %w", err)
		}
	}

	for name, content := range files {
		path := filepath.Join(programDir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0755); err != nil {
			return nil, fmt.Errorf("creating directory for %s: %w", name, err)
		}
		if err := os.WriteFile(path, content, 0644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", name, err)
		}
	}

	if err := writePulumiYaml(req.TargetDirectory, req.Project); err != nil {
		return nil, fmt.Errorf("writing Pulumi.yaml: %w", err)
	}

	// For each parameterized local dependency, store the hcl.sdk.json so that
	// GetRequiredPackages and Run can find the parameterization info later.
	for alias, artifactPath := range req.LocalDependencies {
		data, err := os.ReadFile(filepath.Join(artifactPath, "hcl.sdk.json"))
		if err != nil {
			continue
		}
		var desc workspace.PackageDescriptor
		if err := json.Unmarshal(data, &desc); err != nil {
			continue
		}
		if desc.Parameterization == nil {
			continue
		}
		sdkDir := filepath.Join(programDir, ".hcl", "sdks", alias)
		if err := os.MkdirAll(sdkDir, 0755); err != nil {
			return nil, fmt.Errorf("creating sdk dir for %s: %w", alias, err)
		}
		if err := os.WriteFile(filepath.Join(sdkDir, "hcl.sdk.json"), data, 0644); err != nil {
			return nil, fmt.Errorf("writing hcl.sdk.json for %s: %w", alias, err)
		}
	}

	return &pulumirpc.GenerateProjectResponse{
		Diagnostics: plugin.HclDiagnosticsToRPCDiagnostics(genDiags),
	}, nil
}

// GeneratePackage generates SDK bindings for a Pulumi package.
//
// HCL doesn't need generated SDKs — it uses provider schemas directly. However,
// we write an hcl.sdk.json file containing the package descriptor so that
// GetRequiredPackages can discover which packages a project depends on.
func (host *LanguageHost) GeneratePackage(
	ctx context.Context,
	req *pulumirpc.GeneratePackageRequest,
) (*pulumirpc.GeneratePackageResponse, error) {
	desc, err := packageDescriptorFromSchema([]byte(req.Schema))
	if err != nil {
		return nil, fmt.Errorf("parsing schema for package descriptor: %w", err)
	}

	data, err := json.MarshalIndent(desc, "", "  ")
	if err != nil {
		return nil, fmt.Errorf("marshaling package descriptor: %w", err)
	}
	data = append(data, '\n')

	if err := os.WriteFile(filepath.Join(req.Directory, "hcl.sdk.json"), data, 0644); err != nil {
		return nil, fmt.Errorf("writing hcl.sdk.json: %w", err)
	}

	return &pulumirpc.GeneratePackageResponse{}, nil
}

// packageDescriptorFromSchema extracts a workspace.PackageDescriptor from a JSON
// schema blob. This mirrors the logic in the test framework's interface.go that
// builds expected PackageDescriptors from schema Package definitions.
func packageDescriptorFromSchema(schemaJSON []byte) (workspace.PackageDescriptor, error) {
	var spec schema.PartialPackageSpec
	if err := json.Unmarshal(schemaJSON, &spec); err != nil {
		return workspace.PackageDescriptor{}, fmt.Errorf("unmarshaling schema: %w", err)
	}

	desc := workspace.PackageDescriptor{
		PluginDescriptor: workspace.PluginDescriptor{
			Name:              spec.Name,
			Kind:              apitype.ResourcePlugin,
			PluginDownloadURL: spec.PluginDownloadURL,
		},
	}

	if spec.Version != "" {
		v, err := semver.Parse(spec.Version)
		if err != nil {
			return workspace.PackageDescriptor{}, fmt.Errorf("parsing version %q: %w", spec.Version, err)
		}
		desc.Version = &v
	}

	if spec.Parameterization != nil {
		baseVersion, err := semver.Parse(spec.Parameterization.BaseProvider.Version)
		if err != nil {
			return workspace.PackageDescriptor{}, fmt.Errorf(
				"parsing base provider version %q: %w", spec.Parameterization.BaseProvider.Version, err)
		}
		desc.Parameterization = &workspace.Parameterization{
			Name:    desc.Name,
			Version: *desc.Version,
			Value:   spec.Parameterization.Parameter,
		}
		desc.Name = spec.Parameterization.BaseProvider.Name
		desc.Version = &baseVersion
	}

	return desc, nil
}

// Pack packages an HCL program into a deployable artifact.
func (host *LanguageHost) Pack(
	ctx context.Context,
	req *pulumirpc.PackRequest,
) (*pulumirpc.PackResponse, error) {
	logging.V(5).Infof("Pack: packageDirectory=%s, destinationDirectory=%s",
		req.PackageDirectory, req.DestinationDirectory)

	// Create a named subdirectory within the destination so that multiple packages packed into
	// the same destination directory don't overwrite each other's files.
	pkgName := filepath.Base(req.PackageDirectory)
	artifactPath := filepath.Join(req.DestinationDirectory, pkgName+".sdk")
	if err := os.MkdirAll(artifactPath, 0755); err != nil {
		return nil, fmt.Errorf("creating artifact directory: %w", err)
	}

	if err := fsutil.CopyFile(artifactPath, req.PackageDirectory, nil); err != nil {
		return nil, err
	}

	return &pulumirpc.PackResponse{
		ArtifactPath: artifactPath,
	}, nil
}

// Link links local dependencies into a project. HCL programs don't have
// traditional package management files, so this is a no-op.
func (host *LanguageHost) Link(
	ctx context.Context,
	req *pulumirpc.LinkRequest,
) (*pulumirpc.LinkResponse, error) {
	return &pulumirpc.LinkResponse{}, nil
}

// Ensure LanguageHost implements the interface.
var _ pulumirpc.LanguageRuntimeServer = (*LanguageHost)(nil)

// resourceMonitorAdapter adapts the Pulumi gRPC resource monitor to our interface.
type resourceMonitorAdapter struct {
	monitorClient pulumirpc.ResourceMonitorClient
	engineClient  pulumirpc.EngineClient
	ctx           context.Context
}

// RegisterPackage registers a parameterized package with the engine.
func (r *resourceMonitorAdapter) RegisterPackage(
	ctx context.Context,
	pkg workspace.PackageDescriptor,
) (run.PackageRef, error) {
	versionStr := ""
	if pkg.Version != nil {
		versionStr = pkg.Version.String()
	}
	req := &pulumirpc.RegisterPackageRequest{
		Name:    pkg.Name,
		Version: versionStr,
	}
	if pkg.Parameterization != nil {
		req.Parameterization = &pulumirpc.Parameterization{
			Name:    pkg.Parameterization.Name,
			Version: pkg.Parameterization.Version.String(),
			Value:   pkg.Parameterization.Value,
		}
	}
	resp, err := r.monitorClient.RegisterPackage(ctx, req)
	if err != nil {
		return "", fmt.Errorf("registering package %s: %w", pkg.Name, err)
	}
	return run.PackageRef(resp.Ref), nil
}

// RegisterResource registers a resource with Pulumi.
func (r *resourceMonitorAdapter) RegisterResource(
	ctx context.Context,
	req run.RegisterResourceRequest,
) (*run.RegisterResourceResponse, error) {
	// Convert inputs to protobuf struct
	inputsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Inputs), r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling inputs: %w", err)
	}

	var aliases []*pulumirpc.Alias
	for _, a := range req.Aliases {
		if a.URN != "" {
			aliases = append(aliases, &pulumirpc.Alias{
				Alias: &pulumirpc.Alias_Urn{Urn: a.URN},
			})
		} else if a.Spec != nil {
			spec := &pulumirpc.Alias_Spec{
				Name:    a.Spec.Name,
				Type:    a.Spec.Type,
				Stack:   a.Spec.Stack,
				Project: a.Spec.Project,
			}
			if a.Spec.NoParent {
				spec.Parent = &pulumirpc.Alias_Spec_NoParent{NoParent: true}
			} else if a.Spec.ParentURN != "" {
				spec.Parent = &pulumirpc.Alias_Spec_ParentUrn{ParentUrn: a.Spec.ParentURN}
			}
			aliases = append(aliases, &pulumirpc.Alias{
				Alias: &pulumirpc.Alias_Spec_{Spec: spec},
			})
		}
	}

	// Convert PropertyDependencies to protobuf format
	propDeps := make(map[string]*pulumirpc.RegisterResourceRequest_PropertyDependencies)
	for prop, urns := range req.PropertyDependencies {
		propDeps[prop] = &pulumirpc.RegisterResourceRequest_PropertyDependencies{
			Urns: urns,
		}
	}

	// Build the registration request
	registerReq := &pulumirpc.RegisterResourceRequest{
		Type:                       req.Type,
		Name:                       req.Name,
		Custom:                     req.Custom,
		Remote:                     req.Remote,
		Object:                     inputsStruct,
		Protect:                    &req.Protect,
		Dependencies:               req.Dependencies,
		PropertyDependencies:       propDeps,
		Provider:                   req.Provider,
		Providers:                  req.Providers,
		Parent:                     req.Parent,
		IgnoreChanges:              req.IgnoreChanges,
		Aliases:                    aliases,
		AcceptSecrets:              true,
		AcceptResources:            true,
		SupportsPartialValues:      true,
		DeleteBeforeReplace:        req.DeleteBeforeReplace,
		DeleteBeforeReplaceDefined: req.DeleteBeforeReplaceDef,
		ImportId:                   req.ImportId,
		AdditionalSecretOutputs:    req.AdditionalSecretOutputs,
		RetainOnDelete:             req.RetainOnDelete,
		DeletedWith:                req.DeletedWith,
		ReplaceWith:                req.ReplaceWith,
		HideDiffs:                  req.HideDiffs,
		ReplaceOnChanges:           req.ReplaceOnChanges,
		EnvVarMappings:             req.EnvVarMappings,
		Version:                    req.Version,
		PluginDownloadURL:          req.PluginDownloadURL,
		PackageRef:                 string(req.PackageRef),
	}

	// Add custom timeouts if specified
	if req.CustomTimeouts != nil {
		registerReq.CustomTimeouts = &pulumirpc.RegisterResourceRequest_CustomTimeouts{
			Create: formatTimeoutSeconds(req.CustomTimeouts.Create),
			Update: formatTimeoutSeconds(req.CustomTimeouts.Update),
			Delete: formatTimeoutSeconds(req.CustomTimeouts.Delete),
		}
	}

	// Add replacement trigger if specified
	if !req.ReplacementTrigger.IsNull() {
		trigger, err := plugin.MarshalPropertyValue("replacement_trigger",
			resource.ToResourcePropertyValue(req.ReplacementTrigger),
			r.pluginOptions())
		if err != nil {
			return nil, fmt.Errorf("marshaling replacement trigger: %w", err)
		}
		registerReq.ReplacementTrigger = trigger
	}

	// Call the resource monitor
	resp, err := r.monitorClient.RegisterResource(ctx, registerReq)
	if err != nil {
		return nil, fmt.Errorf("registering resource: %w", err)
	}

	// Unmarshal outputs
	outputs, err := plugin.UnmarshalProperties(resp.Object, r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling outputs: %w", err)
	}

	return &run.RegisterResourceResponse{
		URN:     resp.Urn,
		ID:      resp.Id,
		Outputs: resource.FromResourcePropertyMap(outputs),
	}, nil
}

// Invoke invokes a provider function.
func (r *resourceMonitorAdapter) Invoke(
	ctx context.Context,
	req run.InvokeRequest,
) (*run.InvokeResponse, error) {
	// Convert args to protobuf struct
	argsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Args), r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	// Build the invoke request
	invokeReq := &pulumirpc.ResourceInvokeRequest{
		Tok:               req.Token,
		Args:              argsStruct,
		Provider:          req.Provider,
		Version:           req.Version,
		PluginDownloadURL: req.PluginDownloadURL,
		AcceptResources:   true,
		PackageRef:        string(req.PackageRef),
	}

	// Call the resource monitor
	resp, err := r.monitorClient.Invoke(ctx, invokeReq)
	if err != nil {
		return nil, fmt.Errorf("invoking function: %w", err)
	}

	// Unmarshal return value
	returnVal, err := plugin.UnmarshalProperties(resp.Return, r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return value: %w", err)
	}

	// Convert failures
	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, fmt.Sprintf("%s: %s", f.Property, f.Reason))
	}

	return &run.InvokeResponse{
		Return:   resource.FromResourcePropertyMap(returnVal),
		Failures: failures,
	}, nil
}

// RegisterResourceOutputs registers outputs on a resource (used for stack outputs).
func (r *resourceMonitorAdapter) RegisterResourceOutputs(
	ctx context.Context,
	urn string,
	outputs property.Map,
) error {
	// Convert outputs to protobuf struct
	outputsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(outputs), r.pluginOptions())
	if err != nil {
		return fmt.Errorf("marshaling outputs: %w", err)
	}

	// Call the resource monitor
	_, err = r.monitorClient.RegisterResourceOutputs(ctx, &pulumirpc.RegisterResourceOutputsRequest{
		Urn:     urn,
		Outputs: outputsStruct,
	})
	if err != nil {
		return fmt.Errorf("registering resource outputs: %w", err)
	}

	return nil
}

// CheckPulumiVersion checks if the Pulumi CLI version satisfies the given version range.
func (r *resourceMonitorAdapter) CheckPulumiVersion(ctx context.Context, versionRange string) error {
	// Call the engine's RequirePulumiVersion RPC method
	_, err := r.engineClient.RequirePulumiVersion(ctx, &pulumirpc.RequirePulumiVersionRequest{
		PulumiVersionRange: versionRange,
	})
	return err
}

// Call invokes a method on a resource.
func (r *resourceMonitorAdapter) Call(
	ctx context.Context,
	req run.CallRequest,
) (*run.CallResponse, error) {
	argsStruct, err := plugin.MarshalProperties(resource.ToResourcePropertyMap(req.Args), r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("marshaling args: %w", err)
	}

	resp, err := r.monitorClient.Call(ctx, &pulumirpc.ResourceCallRequest{
		Tok:        req.Token,
		Args:       argsStruct,
		PackageRef: string(req.PackageRef),
	})
	if err != nil {
		return nil, fmt.Errorf("calling method: %w", err)
	}

	returnVal, err := plugin.UnmarshalProperties(resp.Return, r.pluginOptions())
	if err != nil {
		return nil, fmt.Errorf("unmarshaling return value: %w", err)
	}

	var failures []string
	for _, f := range resp.Failures {
		failures = append(failures, fmt.Sprintf("%s: %s", f.Property, f.Reason))
	}

	return &run.CallResponse{
		Return:   resource.FromResourcePropertyMap(returnVal),
		Failures: failures,
	}, nil
}

func (*resourceMonitorAdapter) pluginOptions() plugin.MarshalOptions {
	return plugin.MarshalOptions{
		KeepUnknowns:  true,
		KeepSecrets:   true,
		KeepResources: true,
	}
}

// Ensure resourceMonitorAdapter implements the interface.
var _ run.ResourceMonitor = (*resourceMonitorAdapter)(nil)

// writePulumiYaml writes the Pulumi.yaml file with runtime set to hcl.
func writePulumiYaml(dir string, projectJSON string) error {
	var project map[string]any
	if err := json.Unmarshal([]byte(projectJSON), &project); err != nil {
		return fmt.Errorf("parsing project JSON: %w", err)
	}

	project["runtime"] = "hcl"

	yamlContent, err := yaml.Marshal(project)
	if err != nil {
		return fmt.Errorf("marshaling project to YAML: %w", err)
	}

	path := filepath.Join(dir, "Pulumi.yaml")
	if err := os.WriteFile(path, yamlContent, 0644); err != nil {
		return fmt.Errorf("writing Pulumi.yaml: %w", err)
	}

	return nil
}

// formatTimeoutSeconds converts a timeout in seconds to a duration string.
// Returns empty string if seconds is 0.
func formatTimeoutSeconds(seconds float64) string {
	if seconds == 0 {
		return ""
	}
	return time.Duration(seconds * float64(time.Second)).String()
}
