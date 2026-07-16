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

// Package run implements the HCL program execution engine.
package run

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"math"
	"math/big"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/graph"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi-labs/pulumi-hcl/pkg/potel"
	"github.com/pulumi-labs/pulumi-hcl/pkg/util"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/logging"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	ctyconvert "github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
)

// PackageRef is an opaque reference returned by RegisterPackage that routes
// resource registrations to the correct parameterized provider instance.
type PackageRef string

// ResourceMonitor is the interface for registering resources with Pulumi.
// This matches the resource monitor interface used by the Pulumi engine.
type ResourceMonitor interface {
	// RegisterPackage registers a parameterized package with the engine and returns
	// a PackageRef that must be passed in subsequent resource registrations.
	RegisterPackage(ctx context.Context, pkg workspace.PackageDescriptor) (PackageRef, error)

	// RegisterResource registers a resource with Pulumi.
	RegisterResource(ctx context.Context, req RegisterResourceRequest) (*RegisterResourceResponse, error)

	// ReadResource reads the state of an existing resource. This is the
	// registration form used for stack references, which the engine resolves
	// against the backend rather than creating.
	ReadResource(ctx context.Context, req ReadResourceRequest) (*ReadResourceResponse, error)

	// Invoke invokes a provider function.
	Invoke(ctx context.Context, req InvokeRequest) (*InvokeResponse, error)

	// Call invokes a method on a resource.
	Call(ctx context.Context, req CallRequest) (*CallResponse, error)

	// RegisterResourceOutputs registers outputs on a resource (used for stack outputs).
	RegisterResourceOutputs(ctx context.Context, urn urn.URN, outputs property.Map) error

	// CheckPulumiVersion checks if the Pulumi CLI version satisfies the given version range.
	CheckPulumiVersion(ctx context.Context, versionRange string) error

	// RegisterResourceHook registers a named callback. Must be called before any
	// resource registration that binds the hook by name via Hooks.
	RegisterResourceHook(ctx context.Context, name string, callback ResourceHookFunction, opts ResourceHookOptions) error

	// LogWarning emits a non-fatal warning diagnostic to the engine.
	LogWarning(ctx context.Context, message string) error
}

// ResourceHookFunction is the engine-invoked hook callback. A non-nil return
// fails the operation; the error message is surfaced to the user.
type ResourceHookFunction func(ctx context.Context, args *ResourceHookArgs) error

type ResourceHookOptions struct {
	// OnDryRun controls whether the hook runs during previews.
	OnDryRun bool
}

type ResourceHookArgs struct {
	URN        string
	ID         string
	Name       string
	Type       string
	NewInputs  property.Map
	OldInputs  property.Map
	NewOutputs property.Map
	OldOutputs property.Map
}

type ResourceHookBinding struct {
	BeforeCreate []string
	BeforeUpdate []string
	AfterCreate  []string
	AfterUpdate  []string
	BeforeDelete []string
	AfterDelete  []string
	OnError      []string
}

// CustomTimeouts contains custom timeout values for resource operations.
type CustomTimeouts struct {
	Create float64 // Timeout in seconds for create operations
	Read   float64 // Timeout in seconds for read operations
	Update float64 // Timeout in seconds for update operations
	Delete float64 // Timeout in seconds for delete operations
}

// Alias represents a resource alias - either a URN string or a spec object.
type Alias struct {
	// URN is set for URN-based aliases.
	URN string
	// Spec is set for spec-based aliases.
	Spec *AliasSpec
}

// AliasSpec represents a resource alias specification.
type AliasSpec struct {
	Name      string
	Type      string
	Stack     string
	Project   string
	ParentURN string
	NoParent  bool
}

// RegisterResourceRequest contains the parameters for registering a resource.
type RegisterResourceRequest struct {
	Type                    string
	Name                    string
	Inputs                  property.Map
	Dependencies            []string
	PropertyDependencies    map[string][]string // Map from property key to list of URNs it depends on
	Custom                  bool
	Remote                  bool
	Protect                 bool
	IgnoreChanges           []property.Glob
	Aliases                 []Alias
	Provider                string
	Providers               map[string]string // Map from package name to provider reference (urn::id)
	Parent                  urn.URN
	DeleteBeforeReplace     bool
	DeleteBeforeReplaceDef  bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts          *CustomTimeouts
	ImportId                string // Resource ID to import
	AdditionalSecretOutputs []string
	RetainOnDelete          *bool
	DeletedWith             string          // URN of the resource that, when deleted, causes this resource to be deleted
	ReplaceWith             []string        // URNs of resources whose replacement triggers replacement of this resource
	HideDiffs               []property.Glob // Property paths whose diffs should not be displayed
	ReplaceOnChanges        []property.Glob // Property paths that if changed should force a replacement
	ReplacementTrigger      property.Value  // Value whose change triggers replacement
	EnvVarMappings          map[string]string
	Version                 string
	PluginDownloadURL       string
	PackageRef              PackageRef
	Hooks                   *ResourceHookBinding
}

// RegisterResourceResponse contains the result of registering a resource.
type RegisterResourceResponse struct {
	URN     urn.URN
	ID      string
	Outputs property.Map
}

// ReadResourceRequest contains the parameters for reading an existing resource.
// A read honors only the subset of resource options the engine's ReadResource
// RPC accepts; options that imply lifecycle management (Protect, IgnoreChanges,
// CustomTimeouts, ...) have no meaning for a read and are not carried.
type ReadResourceRequest struct {
	Type                    string
	Name                    string
	ID                      string
	Inputs                  property.Map
	Parent                  urn.URN
	Dependencies            []string
	Provider                string
	Version                 string
	AdditionalSecretOutputs []string
	PluginDownloadURL       string
	PackageRef              PackageRef
}

// ReadResourceResponse contains the result of reading a resource. ID echoes the
// requested ID, since a read identifies the resource rather than minting one.
type ReadResourceResponse struct {
	URN     urn.URN
	ID      string
	Outputs property.Map
}

// InvokeRequest contains the parameters for invoking a function.
type InvokeRequest struct {
	Token             string
	Args              property.Map
	Provider          string
	Version           string
	PluginDownloadURL string
	PackageRef        PackageRef
}

// InvokeResponse contains the result of invoking a function.
type InvokeResponse struct {
	Return   property.Map
	Failures []string
}

// CallRequest contains the parameters for invoking a method on a resource.
type CallRequest struct {
	Token      string
	Args       property.Map
	PackageRef PackageRef
}

// CallResponse contains the result of invoking a method on a resource.
type CallResponse struct {
	Return   property.Map
	Failures []string
}

// moduleInstance represents a single runtime instance of an inlined module.
type moduleInstance struct {
	// Path identifies this instance within the module nesting tree. The
	// leaf [modulepath.Step] carries the count index or for_each key, if
	// any.
	Path modulepath.Path
	// ModuleInfo describes the module call this instance belongs to,
	// shared by every instance of the call.
	ModuleInfo *graph.ModuleInfo
	// Name is the resolved Pulumi logical name of this instance's component:
	// a `pulumi { name = ... }` override when present, else the parent
	// instance's Name joined to this instance's step with ".". It prefixes
	// the derived names of everything inside the instance and is exposed to
	// the instance as pulumi.module.name.
	Name    string
	EvalCtx *eval.Context        // per-instance evaluation context
	URN     urn.URN              // component URN
	Parent  *moduleInstance      // enclosing module instance (nil for root-level calls)
	Index   *int                 // count index (nil if not using count)
	EachKey *cty.Value           // for_each key (nil if not using for_each)
	EachVal *cty.Value           // for_each value (nil if not using for_each)
	mu      sync.Mutex           // protects Outputs
	Outputs map[string]cty.Value // collected output values
}

// outputObject returns the instance's collected outputs as an object value.
func (inst *moduleInstance) outputObject() cty.Value {
	inst.mu.Lock()
	defer inst.mu.Unlock()
	if len(inst.Outputs) == 0 {
		return cty.EmptyObjectVal
	}
	return cty.ObjectVal(inst.Outputs)
}

// instancePath builds the path for one instance of a module call named name
// within the enclosing instance at parentPath, with the leaf step carrying
// the runtime disambiguator (count index or for_each key, if any).
func instancePath(parentPath modulepath.Path, name string, index *int, eachKey *cty.Value) modulepath.Path {
	switch {
	case index != nil:
		return parentPath.Append(modulepath.NewIndexedStep(name, *index))
	case eachKey != nil:
		return parentPath.Append(modulepath.NewKeyedStep(name, eachKey.AsString()))
	default:
		return parentPath.Append(modulepath.NewStep(name))
	}
}

// moduleInstanceName resolves the Pulumi logical name of one module instance:
// a `pulumi { name = ... }` override evaluated in the calling scope (with the
// instance's count.index/each.key in scope) is the full name; a null or
// absent override derives the name from the parent instance's resolved name
// joined to this instance's own step with ".".
func moduleInstanceName(
	mod *ast.Module, parentEvalCtx *eval.Context, parentName string,
	instPath modulepath.Path, index *int, eachKey, eachVal *cty.Value,
) (string, error) {
	if mod.PulumiName != nil {
		hclCtx := parentEvalCtx.HCLContextWithIteration(index, eachKey, eachVal)
		name, ok, err := evaluatePulumiName(mod.PulumiName, hclCtx, "module "+mod.Name)
		if err != nil {
			return "", err
		}
		if ok {
			return name, nil
		}
	}
	_, leaf, ok := instPath.Parent()
	if !ok {
		return "", fmt.Errorf("module %s: instance path is empty", mod.Name)
	}
	return joinModuleName(parentName, leaf.LogicalName()), nil
}

// inheritableOpts holds the resource options that child resources can inherit from their parent.
type inheritableOpts struct {
	Protect        *bool
	RetainOnDelete *bool
}

// unknownTokenDiag turns a packages.NotFoundError or ProviderAsResourceError
// into an hcl.Diagnostic anchored at typeRange. Other errors pass through.
func unknownTokenDiag(kind string, typeRange hcl.Range, err error) error {
	var pae *packages.ProviderAsResourceError
	if errors.As(err, &pae) {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Provider declared as a resource",
			Detail:   pae.Error(),
			Subject:  typeRange.Ptr(),
		}}
	}
	var nfe *packages.NotFoundError
	if !errors.As(err, &nfe) {
		return err
	}
	d := &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("unknown %s type %q", kind, nfe.Token),
		Subject:  typeRange.Ptr(),
	}
	if nfe.Suggestion != "" {
		d.Detail = fmt.Sprintf("did you mean %q?", nfe.Suggestion)
	}
	return hcl.Diagnostics{d}
}

// Engine executes HCL programs against the Pulumi engine.
type Engine struct {
	// config is the parsed HCL configuration.
	config *ast.Config

	// evaluator handles expression evaluation.
	evaluator *eval.Evaluator

	// pkgLoader loads Pulumi package schemas.
	pkgLoader schema.ReferenceLoader

	// providerInfoSource resolves TF provider mappings (bridge ProviderInfo)
	// so HCL block/attribute names line up 1:1 with the TF source. May be nil
	// when the host can't reach a mapper, in which case the convention-based
	// transform path is used.
	providerInfoSource bridge.ProviderInfoSource

	// resolver resolves TF resource/data source types to Pulumi schemas and
	// bridge mappings, shared with schema generation so both resolve identically.
	resolver *packages.Resolver

	// resmon is the resource monitor for registering resources.
	resmon ResourceMonitor

	// resourceOutputs maps resource keys to their output values.
	resourceOutputs *util.SyncMap[string, cty.Value]

	// resourceInheritableOpts maps resource keys to the options that children can inherit.
	resourceInheritableOpts *util.SyncMap[string, inheritableOpts]

	// defaultProviders maps a package name to the urn::id of its un-aliased
	// `provider "<pkg>" {}` block. Resources whose package matches and that
	// omit an explicit `provider` arg use this so the provider block's config
	// flows through, instead of the engine spinning up an empty default.
	defaultProviders *util.SyncMap[string, string]

	// stackOutputs collects outputs to be registered on the stack.
	stackOutputs map[string]property.Value

	// stackURN is the URN of the root stack resource.
	stackURN urn.URN

	// projectName is the current project name.
	projectName string

	// stackName is the current stack name.
	stackName string

	// organization is the current organization name.
	organization string

	// packages maps parameterized package alias to its descriptor, for registration at startup.
	packages map[string]workspace.PackageDescriptor

	// packageRefs maps parameterized package alias to its RegisterPackage ref.
	packageRefs map[string]PackageRef

	// dryRun indicates if this is a preview operation.
	dryRun bool

	// workDir is the working directory.
	workDir string

	// absolutePaths mirrors EngineOptions.AbsolutePaths for the child-module
	// contexts created during the run.
	absolutePaths bool

	// pulumiConfig contains Pulumi stack configuration values.
	pulumiConfig map[string]ConfigValue

	// moduleLoader loads and caches module configurations.
	moduleLoader *modules.Loader

	// moduleInstances maps module path → list of instances for inlined modules.
	moduleInstances *util.SyncMap[modulepath.Path, []*moduleInstance]

	parallel int

	// failedNodes tracks resource nodes that failed to register, keyed by instance key.
	// Dependent nodes check this map and are skipped when a dependency failed.
	failedNodes *util.SyncMap[string, error]

	// graph is the resolved dependency graph for the current run. Stored on
	// the engine so processNode handlers can consult its topology (e.g.
	// processProvider checks whether anything depends on a provider node
	// before registering it).
	graph *graph.Graph

	// forcedCBD holds the resource node keys that must be created before their
	// prior instance is destroyed because they, or a resource depending on
	// them, declared create_before_destroy. Computed once from the graph.
	forcedCBD map[string]bool

	// alwaysRegisterProviders forces every `provider` block to be registered
	// as a resource even when nothing references it, bypassing Terraform's
	// lazy provider-configure semantics. Test-only; see
	// EngineOptions.AlwaysRegisterProviders.
	alwaysRegisterProviders bool
}

// ConfigValue is a value supplied for a root variable. See [UntypedConfigValue] or
// [TypedConfigValue] to create one.
type ConfigValue struct {
	// Exactly one form is set: untyped holds a raw string parsed according to the
	// consuming variable's declared type (the Pulumi config / TF_VAR_ path), while
	// typed holds an already-structured value (the Construct path) whose marks —
	// secrets in particular — are preserved as-is. secret carries the secret bit for
	// an untyped value, whose raw string cannot itself carry a mark; a typed value
	// records its secretness on the value instead.

	untyped *string
	secret  bool
	typed   cty.Value
}

// UntypedConfigValue builds a ConfigValue from a raw string. The string is
// parsed according to the consuming variable's declared type, mirroring how
// OpenTofu parses -var / TF_VAR_ values. secret marks the value sensitive.
func UntypedConfigValue(s string, secret bool) ConfigValue {
	return ConfigValue{untyped: &s, secret: secret}
}

// TypedConfigValue builds a ConfigValue from an already-typed value, preserving
// any marks (such as secrets) it carries. Used when the caller already holds
// structured values, e.g. component Construct inputs.
func TypedConfigValue(v cty.Value) ConfigValue {
	return ConfigValue{typed: v}
}

// EngineOptions configures the engine.
type EngineOptions struct {
	// ProjectName is the Pulumi project name.
	ProjectName string

	// StackName is the Pulumi stack name.
	StackName string

	// Organization is the Pulumi organization name.
	Organization string

	// Config contains the values supplied for root variables, keyed by variable
	// name (optionally project-prefixed)
	Config map[string]ConfigValue

	// DryRun indicates this is a preview operation.
	DryRun bool

	// ResourceMonitor is the resource monitor for registering resources.
	ResourceMonitor ResourceMonitor

	// WorkDir is the working directory (where the program files are).
	WorkDir string

	// RootDir is the project root directory (where Pulumi.yaml is).
	RootDir string

	// AbsolutePaths makes path.module and path.root evaluate to absolute
	// directories. The Construct entry points set it: the module tree lives
	// outside the Pulumi program (a module cache, a bundle unpack dir, or an
	// arbitrary local path) while provider plugins resolve relative paths
	// against the program directory, so the relative values a direct program
	// run renders would point outside the module tree.
	AbsolutePaths bool

	SchemaLoader schema.ReferenceLoader

	// ProviderInfoSource is the bridge mapping resolver. Optional; when nil
	// the engine falls back to convention-based name mapping.
	ProviderInfoSource bridge.ProviderInfoSource

	// Packages maps parameterized package alias to its descriptor.
	// The engine calls RegisterPackage on the resource monitor for each entry before running the program.
	Packages map[string]workspace.PackageDescriptor

	// ModuleLoader loads child module configurations. It must not be nil.
	ModuleLoader *modules.Loader

	Parallel int

	// AlwaysRegisterProviders forces every `provider` block to be registered
	// as a resource even when no resource references it. This exists ONLY for
	// the language conformance tests, whose Pulumi-semantics fixtures expect
	// explicitly-declared providers to appear in the snapshot. Production runs
	// must leave this false to preserve Terraform's lazy provider-configure
	// behavior (an unused provider whose config would fail is never
	// configured).
	AlwaysRegisterProviders bool
}

// NewEngine creates a new execution engine.
func NewEngine(ctx context.Context, config *ast.Config, opts *EngineOptions) (*Engine, error) {
	contract.Requiref(opts.SchemaLoader != nil, "opts.SchemaLoader", "EngineOptions.SchemaLoader cannot be nil")
	contract.Requiref(opts.WorkDir != "", "opts.WorkDir", "EngineOptions.WorkDir cannot be empty")
	contract.Requiref(opts.RootDir != "", "opts.RootDir", "EngineOptions.RootDir cannot be empty")
	contract.Requiref(opts.ModuleLoader != nil, "EngineOptions.ModuleLoader", "cannot be empty")

	evalCtx, err := newEvalContext(opts.AbsolutePaths, opts.WorkDir, opts.RootDir, opts.WorkDir,
		opts.StackName, opts.ProjectName, opts.Organization)
	if err != nil {
		return nil, fmt.Errorf("creating the root evaluation context: %w", err)
	}

	return &Engine{
		config:                  config,
		evaluator:               eval.NewEvaluator(evalCtx),
		pkgLoader:               opts.SchemaLoader,
		providerInfoSource:      opts.ProviderInfoSource,
		resmon:                  opts.ResourceMonitor,
		resourceOutputs:         util.NewSyncMap[string, cty.Value](),
		resourceInheritableOpts: util.NewSyncMap[string, inheritableOpts](),
		defaultProviders:        util.NewSyncMap[string, string](),
		stackOutputs:            make(map[string]property.Value),
		projectName:             opts.ProjectName,
		stackName:               opts.StackName,
		organization:            opts.Organization,
		dryRun:                  opts.DryRun,
		workDir:                 opts.WorkDir,
		absolutePaths:           opts.AbsolutePaths,
		pulumiConfig:            opts.Config,
		packages:                opts.Packages,
		packageRefs:             make(map[string]PackageRef),
		moduleLoader:            opts.ModuleLoader,
		moduleInstances:         util.NewSyncMap[modulepath.Path, []*moduleInstance](),
		parallel:                opts.Parallel,
		failedNodes:             util.NewSyncMap[string, error](),
		alwaysRegisterProviders: opts.AlwaysRegisterProviders,
		resolver: packages.NewResolver(
			opts.SchemaLoader, opts.ProviderInfoSource, opts.Packages, knownProviders(config.Terraform)),
	}, nil
}

func newEvalContext(absolutePaths bool, moduleDir, rootDir, rootModuleDir, stack, project, organization string) (*eval.Context, error) {
	if absolutePaths {
		return eval.NewAbsolutePathContext(moduleDir, rootDir, rootModuleDir, stack, project, organization)
	}
	return eval.NewContext(moduleDir, rootDir, rootModuleDir, stack, project, organization)
}

// Run executes the HCL program.
func (e *Engine) Run(ctx context.Context) error {
	ctx, span := potel.Start(ctx, "Engine.Run")
	defer span.End()
	for alias, pkg := range e.packages {
		ref, err := e.resmon.RegisterPackage(ctx, pkg)
		if err != nil {
			return fmt.Errorf("registering package %s: %w", alias, err)
		}
		e.packageRefs[alias] = ref
	}

	// Register the root stack resource to get its URN for outputs
	if err := e.registerStack(ctx); err != nil {
		return fmt.Errorf("registering stack: %w", err)
	}

	if err := e.installProviderFunctions(ctx, e.evaluator.Context(), e.config, nil); err != nil {
		return err
	}

	// Build the dependency graph with module inlining
	g, err := graph.BuildFromConfig(e.config, &moduleLoaderAdapter{e.moduleLoader}, e.workDir)
	if err != nil {
		return fmt.Errorf("building dependency graph: %w", err)
	}

	// Validate the graph
	if errs := g.Validate(); len(errs) > 0 {
		return errors.Join(errs...)
	}
	e.graph = g
	e.forcedCBD = g.ForcedCreateBeforeDestroy()

	// Process nodes in parallel where possible
	if err := e.processGraph(ctx, g); err != nil {
		return err
	}

	// Collect errors from resources that failed to register but were not fatal
	// (i.e., we continued processing to allow independent resources to proceed).
	nodeErrs := slices.Collect(e.failedNodes.Values())

	// Evaluate check blocks after the program has exited.
	if err := e.evaluateChecks(ctx); err != nil && len(nodeErrs) == 0 {
		return err
	}

	// Process outputs (collect them into stackOutputs). Under continue-on-error
	// some resources failed but recover() can still surface fallback values, so
	// outputs are computed regardless; an output that cannot be computed on a
	// failed run is dropped rather than masking the resource failures below.
	for name, output := range e.config.Outputs {
		if err := e.processOutput(ctx, name, output); err != nil {
			if len(nodeErrs) > 0 {
				continue
			}
			return fmt.Errorf("processing output %s: %w", name, err)
		}
	}

	// Register stack outputs
	if err := e.registerStackOutputs(ctx); err != nil {
		return fmt.Errorf("registering stack outputs: %w", err)
	}

	// Surface resource failures; the engine turns these into a bail under
	// continue-on-error, after the recovered outputs above are registered.
	if len(nodeErrs) > 0 {
		return errors.Join(nodeErrs...)
	}

	return nil
}

// registerStack registers the root stack resource.
func (e *Engine) registerStack(ctx context.Context) error {
	if e.resmon == nil {
		return nil
	}

	stackName := fmt.Sprintf("%s-%s", e.projectName, e.stackName)
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:   "pulumi:pulumi:Stack",
		Name:   stackName,
		Inputs: property.NewMap(nil),
	})
	if err != nil {
		return err
	}

	e.stackURN = resp.URN
	return nil
}

// registerStackOutputs registers all collected outputs on the stack.
func (e *Engine) registerStackOutputs(ctx context.Context) error {
	if e.resmon == nil || len(e.stackOutputs) == 0 {
		return nil
	}

	return e.resmon.RegisterResourceOutputs(ctx, e.stackURN, property.NewMap(e.stackOutputs))
}

// processNode processes a single node based on its type.
func (e *Engine) processNode(ctx context.Context, node *graph.Node) error {
	switch node.Type {
	case graph.NodeTypeVariable:
		return e.processVariable(ctx, node)
	case graph.NodeTypeVariableValidation:
		return e.processVariableValidation(node)
	case graph.NodeTypeLocal:
		return e.processLocal(ctx, node)
	case graph.NodeTypeResource:
		return e.processResource(ctx, node)
	case graph.NodeTypeDataSource:
		return e.processDataSource(ctx, node)
	case graph.NodeTypeModuleInit:
		return e.processModuleInit(ctx, node)
	case graph.NodeTypeModule:
		return e.processModuleComplete(ctx, node)
	case graph.NodeTypeCall:
		return e.processCall(ctx, node)
	case graph.NodeTypeOutput:
		if node.ModuleInfo != nil {
			return e.processModuleOutput(ctx, node)
		}
		return nil
	case graph.NodeTypeProvider:
		return e.processProvider(ctx, node)
	case graph.NodeTypeBuiltin:
		return nil
	case graph.NodeTypeUnknown:
		return errors.New("unknown node type")
	default:
		return fmt.Errorf("unknown node type: %v", node.Type)
	}
}

func (e *Engine) processGraph(ctx context.Context, g *graph.Graph) error {
	if err := g.InjectAfter(e.checkPulumiVersion, func(n *graph.Node) bool {
		return n.Type == graph.NodeTypeVariable && n.ModuleInfo == nil
	}); err != nil {
		return err
	}
	return g.Walk(ctx, e.processNode, e.parallel)
}

// processVariable processes a variable definition.
func (e *Engine) processVariable(ctx context.Context, node *graph.Node) error {
	v := node.Variable
	if v == nil {
		return fmt.Errorf("variable node missing Variable field")
	}

	// Module variable: evaluate input expression in parent context, store in each instance context.
	if node.ModuleInfo != nil {
		return e.processModuleVariable(node)
	}

	varName := v.Name
	var val cty.Value
	var isSecret bool
	var valueSource string

	// Variable value precedence (highest to lowest):
	// 1. Environment variable TF_VAR_<name>
	// 2. Pulumi stack config (projectName:<name>)
	// 3. Default value

	if e.evaluator.Context().HCLContext().Variables["var"].Type().HasAttribute(varName) {
		return fmt.Errorf("%q already evaluated", varName)
	}

	// Check environment variable first
	envVarName := "TF_VAR_" + varName
	if envVal := os.Getenv(envVarName); envVal != "" {
		val = cty.StringVal(envVal)
		valueSource = "environment"
	} else if e.pulumiConfig != nil {
		// Check Pulumi stack config, preferring the project-prefixed key.
		configKey := e.projectName + ":" + varName
		cv, ok := e.pulumiConfig[configKey]
		if !ok {
			cv, ok = e.pulumiConfig[varName]
		}
		if ok {
			if cv.untyped != nil {
				// A raw string parsed below according to the declared type.
				val = cty.StringVal(*cv.untyped)
				valueSource = "config"
				isSecret = cv.secret
			} else {
				// An already-typed value; its marks (e.g. secrets) ride along,
				// so it bypasses string parsing.
				val = cv.typed
				valueSource = "config-typed"
			}
		}
	}

	// If no value from env or config, use default. A variable without a default
	// is required, regardless of its `nullable` setting (which only governs
	// whether a *provided* value may be the null literal).
	if valueSource == "" {
		if v.Default != nil {
			var diags hcl.Diagnostics
			val, diags = e.evaluator.EvaluateExpression(v.Default)
			if diags.HasErrors() {
				return fmt.Errorf("evaluating variable default: %s", diags.Error())
			}
			valueSource = "default"
		} else {
			return fmt.Errorf("variable %q is required but no value was provided. Set it with TF_VAR_%s environment variable or Pulumi config: pulumi config set %s <value>",
				varName, varName, varName)
		}
	}

	// A value from a string source (env/config) is parsed according to the declared
	// type. A variable declared without a type keeps its literal string value,
	// matching OpenTofu's VariableParseLiteral default; one declared with any type
	// — including `any` — parses its value as HCL (VariableParseHCL).
	if (valueSource == "environment" || valueSource == "config") &&
		v.TypeConstraint != cty.NilType {
		converted, err := convertStringToType(val.AsString(), v.TypeConstraint)
		if err != nil {
			return fmt.Errorf("variable %q: %w", varName, err)
		}
		val = converted
	}

	// Fill in optional()-attribute defaults before sensitive marking.
	if v.TypeDefaults != nil && !val.IsNull() {
		val = v.TypeDefaults.Apply(val)
	}

	if valueSource != "environment" && valueSource != "config" &&
		v.TypeConstraint != cty.NilType && v.TypeConstraint != cty.DynamicPseudoType {
		if converted, err := ctyconvert.Convert(val, v.TypeConstraint); err == nil {
			val = converted
		}
	}

	// A resource reference supplied as a typed config value — e.g. a component
	// input from a calling program — carries only its identity. Fetch the
	// referenced resource's state so the program can read its fields.
	if valueSource == "config-typed" {
		if u, ok := eval.ResourceReferenceURN(val); ok {
			resolved, err := e.resolveConfigResourceReference(ctx, val, u)
			if err != nil {
				return fmt.Errorf("variable %q: %w", varName, err)
			}
			val = resolved
		}
	}

	// Handle sensitive marking
	if v.Sensitive || isSecret {
		val = val.Mark(eval.SensitiveMark)
	}
	if v.Ephemeral {
		val = val.Mark(eval.EphemeralMark)
	}

	// Store in eval context (needed for validation which may reference var.<name>)
	e.evaluator.Context().SetVariable(varName, val)

	return runVariableValidations(e.evaluator, varName, v.Validations)
}

// processVariableValidation runs the validation rules of a variable whose
// rules reference other objects (e.g. a resource's computed output). The rules
// live on their own graph node, ordered after both the variable's value and
// the referenced objects, so consumers of the variable only observe a
// validated value.
func (e *Engine) processVariableValidation(node *graph.Node) error {
	v := node.Variable
	if v == nil {
		return fmt.Errorf("variable validation node missing Variable field")
	}
	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			return runVariableValidations(eval.NewEvaluator(inst.EvalCtx), v.Name, v.Validations)
		})
	}
	return runVariableValidations(e.evaluator, v.Name, v.Validations)
}

// runVariableValidations evaluates a variable's `validation` rules against ev
// (whose context must already hold the variable's value). It returns an error
// for the first rule whose condition is known and false; unknown conditions are
// deferred.
func runVariableValidations(ev *eval.Evaluator, varName string, validations []*ast.Validation) error {
	for i, validation := range validations {
		condVal, diags := ev.EvaluateExpression(validation.Condition)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating validation condition %d for variable %q: %s", i+1, varName, diags.Error())
		}
		condVal, _ = condVal.Unmark()
		if !condVal.IsKnown() {
			continue
		}

		condOK, err := conditionResultToBool(condVal)
		if err != nil {
			return fmt.Errorf("validation condition %d for variable %q: %s", i+1, varName, err)
		}

		if !condOK {
			errMsgVal, diags := ev.EvaluateExpression(validation.ErrorMessage)
			errMsg := "validation failed"
			if !diags.HasErrors() {
				if s := renderErrorMessage(errMsgVal); s != "" {
					errMsg = s
				}
			}
			return fmt.Errorf("validation failed for variable %q: %s", varName, errMsg)
		}
	}

	return nil
}

// convertStringToType converts a string-sourced variable value (Pulumi config
// or a TF_VAR_ environment variable) to the variable's declared type.
func convertStringToType(s string, targetType cty.Type) (cty.Value, error) {
	var val cty.Value
	if targetType.IsPrimitiveType() {
		val = cty.StringVal(s)
	} else {
		expr, diags := hclsyntax.ParseExpression([]byte(s), "<variable value>", hcl.Pos{Line: 1, Column: 1})
		if diags.HasErrors() {
			return cty.NilVal, fmt.Errorf("cannot parse %q as an HCL expression: %s", s, diags.Error())
		}
		v, valDiags := expr.Value(nil)
		if valDiags.HasErrors() {
			return cty.NilVal, fmt.Errorf("cannot evaluate %q: %s", s, valDiags.Error())
		}
		val = v
	}

	converted, err := ctyconvert.Convert(val, targetType)
	if err != nil {
		return cty.NilVal, fmt.Errorf("cannot convert %q to %s: %w", s, targetType.FriendlyName(), err)
	}
	return converted, nil
}

// processLocal processes a local value definition.
func (e *Engine) processLocal(ctx context.Context, node *graph.Node) error {
	local := node.Local
	if local == nil {
		return fmt.Errorf("local node missing Local field")
	}

	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			localName := strings.TrimPrefix(node.Key, node.ModuleInfo.Prefix()+"local.")
			val, diags := local.Value.Value(inst.EvalCtx.HCLContext())
			if diags.HasErrors() {
				return fmt.Errorf("evaluating local value %s: %s", localName, diags.Error())
			}
			inst.EvalCtx.SetLocal(localName, val)
			return nil
		})
	}

	val, diags := e.evaluator.EvaluateExpression(local.Value)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating local value: %s", diags.Error())
	}

	localName := node.Key[6:] // Remove "local." prefix
	e.evaluator.Context().SetLocal(localName, val)

	return nil
}

// processProvider processes a provider configuration and registers it as a provider resource.
func (e *Engine) processProvider(ctx context.Context, node *graph.Node) error {
	provider := node.Provider
	if provider == nil {
		return fmt.Errorf("provider node missing Provider field")
	}

	// TF/tofu only configures providers that something actually uses; an
	// unused `provider` block is silently ignored even if its body would
	// fail validate/configure. The graph already captures every real use
	// as an in-edge to the provider node (explicit `provider = ...` refs
	// and root implicit-default refs), so no dependents means no use.
	// alwaysRegisterProviders (test-only) opts out so conformance fixtures
	// see explicitly-declared providers in the snapshot.
	if !e.alwaysRegisterProviders && e.graph != nil && !e.graph.HasDependents(node.Key) {
		return nil
	}

	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			return e.registerProvider(ctx, node, provider, inst.EvalCtx, inst.URN, inst)
		})
	}

	return e.registerProvider(ctx, node, provider, e.evaluator.Context(), e.stackURN, nil)
}

// providerInstance carries one `for_each` instance of a provider block: the
// instance key and the element value bound to each.key/each.value while the
// block's config is evaluated.
type providerInstance struct {
	key   string
	value cty.Value
}

// registerProvider registers a provider block in evalCtx: one instance per
// for_each key when the block has `for_each`, otherwise a single
// configuration.
func (e *Engine) registerProvider(
	ctx context.Context, node *graph.Node, provider *ast.Provider,
	evalCtx *eval.Context, parentURN urn.URN, modInst *moduleInstance,
) error {
	if provider.ForEach == nil {
		return e.registerProviderInContext(ctx, node, provider, evalCtx, parentURN, modInst, nil)
	}

	forEach, unknown, _, diags := eval.NewEvaluator(evalCtx).EvaluateForEach(provider.ForEach)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating for_each for provider %s: %s", node.Key, diags.Error())
	}
	// Provider instances must be configurable up front, so unlike a
	// resource's for_each an unknown value is an error even during preview.
	if unknown {
		return fmt.Errorf("%s: the for_each value depends on values that are not yet known", node.Key)
	}

	for _, key := range slices.Sorted(maps.Keys(forEach)) {
		inst := &providerInstance{key: key, value: forEach[key]}
		if err := e.registerProviderInContext(ctx, node, provider, evalCtx, parentURN, modInst, inst); err != nil {
			return err
		}
	}

	// An empty for_each still binds the provider address so a reference
	// evaluates to an empty collection rather than "no such attribute".
	if len(forEach) == 0 {
		baseKey := node.Key
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		evalCtx.SetResource(baseKey, "", cty.EmptyObjectVal)
	}

	return nil
}

// resolvePassThroughProvider looks up a provider passed into a module via
// `providers = { <localKey> = <parentExpr> }` and returns the resolved
// URN::ID, or "" when the resource isn't in a module, there's no entry for
// localKey, or the parent expression doesn't yield a provider reference. An
// expression the parent's scope doesn't bind is chased recursively through
// the parent's own pass-through entries.
func (e *Engine) resolvePassThroughProvider(modInfo *graph.ModuleInfo, localKey string) string {
	if modInfo == nil || modInfo.Module == nil || localKey == "" {
		return ""
	}
	passExpr, ok := modInfo.Module.Providers[localKey]
	if !ok {
		return ""
	}
	if parentCtx := e.parentEvalContext(modInfo); parentCtx != nil {
		val, diags := eval.NewEvaluator(parentCtx).EvaluateExpression(passExpr)
		if !diags.HasErrors() {
			if ref, err := providerRefFromCty(val); err == nil {
				return ref
			}
		}
	}
	return e.resolvePassThroughProvider(e.parentModuleInfo(modInfo), providerExprKey(passExpr))
}

// parentModuleInfo returns the ModuleInfo of the module call enclosing
// modInfo, or nil when the parent is the root config (or has no instances).
func (e *Engine) parentModuleInfo(modInfo *graph.ModuleInfo) *graph.ModuleInfo {
	if modInfo.ParentPrefix() == "" {
		return nil
	}
	insts, ok := e.moduleInstances.Get(modInfo.ParentPath())
	if !ok || len(insts) == 0 {
		return nil
	}
	return insts[0].ModuleInfo
}

// resolveExplicitProvider resolves a resource/data source `provider = ...`
// expression to a "<urn>::<id>" reference. A pass-through entry of the
// instantiating module call wins over any local definition; otherwise the
// expression is evaluated in the resource's own scope. A bare `provider =
// name` reference whose default configuration is registered nowhere in scope
// resolves to "": the reference names the implicit empty default
// configuration, i.e. the engine default.
func (e *Engine) resolveExplicitProvider(
	expr hcl.Expression, evalCtx *eval.Context, modInfo *graph.ModuleInfo,
) (string, error) {
	if ref := e.resolvePassThroughProvider(modInfo, providerExprKey(expr)); ref != "" {
		return ref, nil
	}
	val, valDiags := eval.NewEvaluator(evalCtx).EvaluateExpression(expr)
	if !valDiags.HasErrors() {
		return providerRefFromCty(val)
	}
	vars := expr.Variables()
	if len(vars) != 1 || len(vars[0]) != 1 {
		return "", errors.New(valDiags.Error())
	}
	name := vars[0].RootName()
	if modInfo != nil {
		if ref := e.inheritedDefaultProvider(modInfo, name); ref != "" {
			return ref, nil
		}
		return "", nil
	}
	if ref, ok := e.defaultProviders.Get(e.providerPackageName(name)); ok {
		return ref, nil
	}
	return "", nil
}

// inheritedDefaultProvider walks up the module tree from modInfo, returning
// the nearest ancestor's un-aliased default provider config for pkg
// (URN::ID), or "" if none. An ancestor's default is its own registered block
// or one passed to it through its module call. The graph adds a matching edge
// so that block is registered before this resolves.
func (e *Engine) inheritedDefaultProvider(modInfo *graph.ModuleInfo, pkg string) string {
	for path := modInfo.Path; ; {
		parent, _, ok := path.Parent()
		if !ok {
			return ""
		}
		if outputs, ok := e.resourceOutputs.Get(parent.PrefixString() + pkg); ok {
			if ref, err := providerRefFromCty(outputs); err == nil {
				return ref
			}
		}
		if insts, ok := e.moduleInstances.Get(parent); ok && len(insts) > 0 {
			if ref := e.resolvePassThroughProvider(insts[0].ModuleInfo, pkg); ref != "" {
				return ref
			}
		}
		path = parent
	}
}

// parentEvalContext returns the eval.Context of the enclosing module
// instance (or the root context when modInfo's parent is root).
func (e *Engine) parentEvalContext(modInfo *graph.ModuleInfo) *eval.Context {
	if modInfo.ParentPrefix() == "" {
		return e.evaluator.Context()
	}
	parentInsts, ok := e.moduleInstances.Get(modInfo.ParentPath())
	if !ok || len(parentInsts) == 0 {
		return nil
	}
	return parentInsts[0].EvalCtx
}

// providerExprKey returns "name" or "name.alias" from a provider-reference
// expression. Returns "" for anything that isn't a single one-or-two-step
// traversal. Mirrors graph.providerExprKey — duplicated to keep the run
// package free of internal graph helpers.
func providerExprKey(expr hcl.Expression) string {
	if expr == nil {
		return ""
	}
	traversals := expr.Variables()
	if len(traversals) != 1 {
		return ""
	}
	t := traversals[0]
	if len(t) == 0 {
		return ""
	}
	name := t.RootName()
	if len(t) == 1 {
		return name
	}
	if attr, ok := t[1].(hcl.TraverseAttr); ok {
		return name + "." + attr.Name
	}
	return name
}

func (e *Engine) registerProviderInContext(
	ctx context.Context, node *graph.Node, provider *ast.Provider,
	evalCtx *eval.Context, parentURN urn.URN, modInst *moduleInstance,
	inst *providerInstance,
) error {
	pkgName := e.providerPackageName(provider.Name)
	typeToken := "pulumi:providers:" + pkgName

	if inst != nil {
		key := cty.StringVal(inst.key)
		evalCtx = evalCtx.WithIteration(nil, &key, &inst.value)
	}

	hclCtx := evalCtx.HCLContext()

	// Schema-aware eval is needed so schema.Property.Secret marks survive.
	pkg, perr := packages.ResolvePackage(ctx, e.pkgLoader, knownProviders(e.config.Terraform), "pulumi_providers_"+pkgName)
	if perr != nil {
		return fmt.Errorf("resolving provider package %s: %w", provider.Name, perr)
	}
	resSchema, perr := pkg.Provider()
	if perr != nil {
		return fmt.Errorf("resolving provider schema for %s: %w", provider.Name, perr)
	}

	providerMapping := e.resolver.ProviderConfigBodyMapping(ctx, pkgName)
	inputsMap, _, diags := transform.EvalResourceWithSchema(provider.Config, resSchema, providerMapping,
		func(_ resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
			c := hclCtx
			if len(extraVars) > 0 {
				child := hclCtx.NewChild()
				child.Variables = extraVars
				c = child
			}
			return expr.Value(c)
		})
	if diags.HasErrors() {
		return fmt.Errorf("evaluating provider %s config: %s", provider.Name, diags.Error())
	}
	inputs := make(map[string]property.Value, inputsMap.Len())
	inputsMap.AllStable(func(k string, v property.Value) bool {
		inputs[k] = v
		return true
	})

	logicalName := provider.Alias
	if logicalName == "" {
		logicalName = provider.Name
	}
	if inst != nil {
		logicalName = buildResourceName(logicalName, nil, &inst.key)
	}
	if modInst != nil {
		logicalName = joinModuleName(modInst.Name, logicalName)
	}

	// Version comes from an explicit attribute, else required_providers.
	var version string
	if provider.Version != nil {
		val, vdiags := provider.Version.Value(hclCtx)
		if vdiags.HasErrors() {
			return fmt.Errorf("evaluating provider version: %s", vdiags.Error())
		}
		if val.Type() == cty.String {
			version = val.AsString()
		}
	}
	if version == "" && e.config.Terraform != nil {
		if req, ok := e.config.Terraform.RequiredProviders[provider.Name]; ok && req.IsPulumi() {
			version = req.Version
		}
	}

	req := RegisterResourceRequest{
		Type:       typeToken,
		Name:       logicalName,
		Custom:     true,
		Parent:     parentURN,
		Version:    version,
		PackageRef: e.packageRefs[pkgName],
	}
	if resSchema.PackageReference != nil {
		req.PluginDownloadURL = resSchema.PackageReference.PluginDownloadURL()
	}

	if provider.EnvVarMappings != nil {
		val, vdiags := provider.EnvVarMappings.Value(hclCtx)
		if vdiags.HasErrors() {
			return fmt.Errorf("evaluating env_var_mappings: %s", vdiags.Error())
		}
		mappings, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting env_var_mappings: %w", err)
		}
		if mappings.IsMap() {
			req.EnvVarMappings = make(map[string]string)
			mappings.AsMap().AllStable(func(k string, v property.Value) bool {
				if v.IsString() {
					req.EnvVarMappings[k] = v.AsString()
				}
				return true
			})
		}
	}

	if provider.PluginDownloadURL != nil {
		val, vdiags := provider.PluginDownloadURL.Value(hclCtx)
		if vdiags.HasErrors() {
			return fmt.Errorf("evaluating plugin_download_url: %s", vdiags.Error())
		}
		if val.Type() == cty.String {
			// Surface via Inputs so it appears as a top-level output;
			// the resource-option field (req.PluginDownloadURL) is
			// set below to the package's schema default.
			inputs["pluginDownloadURL"] = property.New(val.AsString())
		}
	}

	if provider.AdditionalSecretOutputs != nil {
		val, vdiags := provider.AdditionalSecretOutputs.Value(hclCtx)
		if vdiags.HasErrors() {
			return fmt.Errorf("evaluating additional_secret_outputs: %s", vdiags.Error())
		}
		if val.Type().IsTupleType() || val.Type().IsListType() {
			for it := val.ElementIterator(); it.Next(); {
				_, v := it.Element()
				if v.Type() == cty.String {
					req.AdditionalSecretOutputs = append(req.AdditionalSecretOutputs, v.AsString())
				}
			}
		}
	}

	req.Inputs = property.NewMap(inputs)

	resp, err := e.resmon.RegisterResource(ctx, req)
	if err != nil {
		return fmt.Errorf("registering provider %s: %w", node.Key, err)
	}

	providerID := resp.ID

	// Use ResourceOutputToCty to match processResource's snake_case key emission.
	outputObj, err := transform.ResourceOutputToCty(resp.Outputs, resSchema, providerMapping, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting provider outputs to HCL types: %w", err)
	}
	if e.dryRun && providerID == "" {
		outputObj["id"] = cty.UnknownVal(cty.String)
	} else {
		outputObj["id"] = cty.StringVal(providerID)
	}
	// Providers are resource references that `provider = X.alias` resolves
	// through the eval context, so their `urn` must stay visible (it is not
	// marked synthetic). Unlike a managed resource, a provider can't be
	// referenced as a value in HCL, so there's no user-facing iteration to leak
	// into.
	outputObj["urn"] = cty.StringVal(string(resp.URN))

	outputsKey := node.Key
	if inst != nil {
		outputsKey = fmt.Sprintf("%s[%q]", node.Key, inst.key)
	}
	e.resourceOutputs.Set(outputsKey, cty.ObjectVal(outputObj).Mark(eval.DepMark(resp.URN)))

	// Top-level un-aliased provider blocks become the default provider for
	// resources of the same package that don't set `provider` explicitly.
	if provider.Alias == "" && node.ModuleInfo == nil && providerID != "" {
		e.defaultProviders.Set(pkgName, string(resp.URN)+"::"+providerID)
	}

	markedProviderOutputs := cty.ObjectVal(outputObj).Mark(eval.DepMark(resp.URN))
	bareKey := node.Key
	if node.ModuleInfo != nil {
		// Strip prefix for module-internal references
		bareKey = strings.TrimPrefix(node.Key, node.ModuleInfo.Prefix())
	}
	if inst != nil {
		// Instances assemble into an object keyed by each.key, so
		// `<name>.<alias>["key"]` selects one through ordinary indexing.
		evalCtx.SetEachResource(bareKey, inst.key, resp.URN, markedProviderOutputs)
	} else {
		evalCtx.SetResource(bareKey, resp.URN, markedProviderOutputs)
	}

	return nil
}

// processResource processes a resource definition.
func (e *Engine) processResource(ctx context.Context, node *graph.Node) error {
	res := node.Resource
	if res == nil {
		return fmt.Errorf("resource node missing Resource field")
	}

	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			return e.processResourceInContext(ctx, node, res, inst.EvalCtx, inst.URN, inst)
		})
	}

	return e.processResourceInContext(ctx, node, res, e.evaluator.Context(), e.stackURN, nil)
}

func (e *Engine) processResourceInContext(
	ctx context.Context, node *graph.Node, res *ast.Resource,
	evalCtx *eval.Context, parentURN urn.URN, modInst *moduleInstance,
) error {
	resSchema, err := e.resolver.ResolveResource(ctx, res.Type)
	if err != nil {
		if diag := unknownTokenDiag("resource", res.TypeRange, err); diag != err {
			return diag
		}
		return fmt.Errorf("resolving resource type %s: %w", res.Type, err)
	}

	tempEvaluator := eval.NewEvaluator(evalCtx)

	expander := graph.NewResourceExpander()

	// A reference in count/for_each establishes a dependency (as in TF) that
	// governs destroy ordering, even when nothing in the body references the
	// target. Collect those references so every instance depends on them.
	var metaArgDeps []string
	unknownArg := ""
	if res.Count != nil {
		count, isBool, unknown, deps, diags := tempEvaluator.EvaluateCount(res.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating count: %s", diags.Error())
		}
		metaArgDeps = append(metaArgDeps, deps...)
		switch {
		case unknown:
			unknownArg = "count"
		case isBool:
			expander.SetBoolCount(node.Key, count)
		default:
			expander.SetCount(node.Key, count)
		}
	}

	if res.ForEach != nil {
		forEach, unknown, deps, diags := tempEvaluator.EvaluateForEach(res.ForEach)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating for_each: %s", diags.Error())
		}
		metaArgDeps = append(metaArgDeps, deps...)
		if unknown {
			unknownArg = "for_each"
		} else {
			expander.SetForEach(node.Key, forEach)
		}
	}

	// A count/for_each that reads values this operation has not yet produced
	// cannot be expanded. During preview, register no instances and bind the
	// resource address to unknown so downstream references resolve to unknown
	// rather than an empty collection.
	if unknownArg != "" {
		if !e.dryRun {
			return fmt.Errorf("%s: the %s value depends on values that are not yet known", node.Key, unknownArg)
		}
		baseKey := node.Key
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		evalCtx.SetResource(baseKey, "", cty.UnknownVal(cty.DynamicPseudoType))
		return nil
	}

	result := expander.Expand(node)

	for _, instance := range result.Instances {
		if e.hasFailedDependency(res) {
			e.failedNodes.Set(instance.Key, fmt.Errorf("skipped: dependency failed"))
			continue
		}
		if err := e.registerResourceInstanceInContext(
			ctx, node, res, resSchema, instance, evalCtx, parentURN, modInst, metaArgDeps,
		); err != nil {
			return fmt.Errorf("registering %s: %w", instance.Key, err)
		}
	}

	// Empty count/for_each still needs the resource address bound so downstream
	// references (e.g. an unconditional module output that walks the resource)
	// see an empty collection rather than "no such attribute".
	if len(result.Instances) == 0 && (res.Count != nil || res.ForEach != nil) {
		baseKey := node.Key
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		var empty cty.Value
		if res.Count != nil {
			empty = cty.EmptyTupleVal
		} else {
			empty = cty.EmptyObjectVal
		}
		evalCtx.SetResource(baseKey, "", empty)
	}

	return nil
}

// registerResourceInstanceInContext registers a single resource instance with Pulumi.
func (e *Engine) registerResourceInstanceInContext(
	ctx context.Context,
	node *graph.Node,
	res *ast.Resource,
	resSchema *schema.Resource,
	instance *graph.ExpandedResource,
	evalCtx *eval.Context,
	parentURN urn.URN,
	modInst *moduleInstance,
	metaArgDeps []string,
) error {
	evalCtx = evalCtx.WithIteration(instance.Index, instance.EachKey, instance.EachValue)

	hclCtx := evalCtx.HCLContext()

	dependsOn := make(map[string][]string)
	addToDependsOn := func(prop, urn string) {
		idx, found := slices.BinarySearch(dependsOn[prop], urn)
		if found {
			return
		}
		dependsOn[prop] = slices.Insert(dependsOn[prop], idx, urn)
	}

	plainInputProps := make(map[string]bool, len(resSchema.InputProperties))
	for _, p := range resSchema.InputProperties {
		plainInputProps[p.Name] = p.Plain
	}

	resourceMapping := e.resolver.ResourceBodyMapping(ctx, res.Type)
	// terraform_data's attributes are dynamically typed, so their cty types
	// (e.g. set-ness) survive the Pulumi property round-trip only via the
	// evaluated values captured here; see wrapTerraformDataInputs.
	var tdataEvaluated map[string]cty.Value
	if res.Type == packages.TerraformDataType {
		tdataEvaluated = map[string]cty.Value{}
	}
	resourceInputs, ephemeralPaths, diags := transform.EvalResourceWithSchema(res.Config, resSchema, resourceMapping,
		func(propKey resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
			var val cty.Value
			var diags hcl.Diagnostics
			if len(extraVars) > 0 {
				childCtx := hclCtx.NewChild()
				childCtx.Variables = extraVars
				val, diags = expr.Value(childCtx)
			} else {
				val, diags = expr.Value(hclCtx)
			}
			if diags.HasErrors() {
				return val, diags
			}

			if tdataEvaluated != nil {
				tdataEvaluated[string(propKey)] = val
			}

			if plainInputProps[string(propKey)] {
				return val, diags
			}

			for _, urn := range eval.CollectDepURNs(val) {
				addToDependsOn(string(propKey), urn)
			}

			return val, diags
		})
	if diags.HasErrors() {
		return diags
	}
	if strings.HasPrefix(res.Type, "pulumi_providers_") && res.PluginDownloadURL != nil {
		val, valDiags := res.PluginDownloadURL.Value(hclCtx)
		if !valDiags.HasErrors() && val.Type() == cty.String {
			resourceInputs = resourceInputs.Set("pluginDownloadURL", property.New(val.AsString()))
		}
	}

	opts, err := e.buildResourceOptionsInContext(ctx, res, instance, evalCtx, parentURN, node.ModuleInfo, modInst, resourceMapping, resSchema.InputProperties, resSchema.Properties, resourceInputs)
	if err != nil {
		return err
	}
	// The options are built from the unboxed inputs: terraform_data's
	// {type, value} boxing would hide the value shapes that
	// ignoreChangesApplies inspects.
	resourceInputs = wrapTerraformDataInputs(res.Type, resourceInputs, tdataEvaluated)
	opts.Custom = !resSchema.IsComponent
	opts.Remote = resSchema.IsComponent
	opts.PropertyDependencies = dependsOn

	// An ephemeral value is free to differ on every run, so a property it
	// flows into should not display a diff. The path arrives in the assembled
	// inputs' namespace (snake-cased Pulumi names, MaxItemsOne blocks already
	// flattened), which translateAttrPathTraversal maps to engine form.
	for _, p := range ephemeralPaths {
		glob, err := translateAttrPathTraversal(ctyPathTraversal(p), resourceMapping, resSchema.InputProperties)
		if err != nil {
			return fmt.Errorf("translating ephemeral property path on %q: %w", res.Type+"."+res.Name, err)
		}
		if !slices.Contains(opts.HideDiffs, glob) {
			opts.HideDiffs = append(opts.HideDiffs, glob)
		}
	}
	for _, deps := range dependsOn {
		for _, dep := range deps {
			if !slices.Contains(opts.DependsOn, dep) {
				opts.DependsOn = append(opts.DependsOn, dep)
			}
		}
	}
	// count/for_each, lifecycle precondition/postcondition, and
	// provisioner/connection references are not tied to any body property, so
	// they stay out of PropertyDependencies but still gate ordering through
	// DependsOn.
	checkDeps := checkRuleDeps(res.Preconditions, hclCtx)
	checkDeps = append(checkDeps, checkRuleDeps(res.Postconditions, hclCtx)...)
	provDeps := provisionerDeps(res, hclCtx)
	for _, dep := range slices.Concat(metaArgDeps, checkDeps, provDeps) {
		if !slices.Contains(opts.DependsOn, dep) {
			opts.DependsOn = append(opts.DependsOn, dep)
		}
	}
	slices.Sort(opts.DependsOn)

	if opts.Version == "" {
		pkgName := packageNameFromResourceType(res.Type)
		if e.config.Terraform != nil {
			if req, ok := e.config.Terraform.RequiredProviders[pkgName]; ok && req.IsPulumi() {
				opts.Version = req.Version
			}
		}
	}

	if opts.PluginDownloadURL == "" && resSchema.PackageReference != nil {
		opts.PluginDownloadURL = resSchema.PackageReference.PluginDownloadURL()
	}

	opts.PackageRef = e.packageRefForResource(res.Type, resSchema)

	resourceName, err := e.resourceInstanceName(res, instance, hclCtx, modInst)
	if err != nil {
		return err
	}

	if len(res.Preconditions) > 0 {
		if err := e.bindPreconditionHooks(ctx, res, instance, evalCtx, opts, resourceName); err != nil {
			return err
		}
	}

	if len(res.Postconditions) > 0 {
		if err := e.bindPostconditionHooks(ctx, res, resSchema, resourceMapping, instance, evalCtx, opts, resourceName); err != nil {
			return err
		}
	}

	if len(res.Provisioners) > 0 {
		if err := e.bindProvisionerHooks(ctx, res, resSchema, resourceMapping, instance, evalCtx, opts, resourceName); err != nil {
			return err
		}
	}

	urn, id, outputs, err := e.registerResource(ctx, res.Type, resSchema.Token, resourceName, resourceInputs, opts)
	if err != nil {
		e.failedNodes.Set(instance.Key, fmt.Errorf("registering resource: %w", err))
		return nil
	}

	outputs = outputs.Delete("id", "urn")

	outputs, err = e.resolveResourceRefsInOutputs(ctx, outputs, resSchema)
	if err != nil {
		return fmt.Errorf("resolving resource references in outputs: %w", err)
	}

	outputObj, err := transform.ResourceOutputToCty(outputs, resSchema, resourceMapping, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting resource outputs to HCL types: %w", err)
	}
	if err := unwrapTerraformDataOutputs(res.Type, outputObj, outputs); err != nil {
		return fmt.Errorf("converting resource outputs to HCL types: %w", err)
	}
	if e.dryRun && id == "" {
		outputObj["id"] = cty.UnknownVal(cty.String)
	} else {
		outputObj["id"] = cty.StringVal(id)
	}
	outputObj["urn"] = cty.StringVal(string(urn)).Mark(eval.SyntheticMark)

	markedOutputs := cty.ObjectVal(outputObj).Mark(eval.DepMark(urn))

	e.resourceOutputs.Set(instance.Key, markedOutputs)

	var iOpts inheritableOpts
	if opts.Protect {
		iOpts.Protect = ptr(true)
	}
	iOpts.RetainOnDelete = opts.RetainOnDelete
	e.resourceInheritableOpts.Set(instance.Key, iOpts)

	if instance.Index != nil {
		baseKey := instance.OriginalKey
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		evalCtx.SetCountResource(baseKey, *instance.Index, urn, markedOutputs)
	} else if instance.EachKey != nil {
		baseKey := instance.OriginalKey
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		evalCtx.SetEachResource(baseKey, instance.EachKey.AsString(), urn, markedOutputs)
	} else if node.ModuleInfo != nil {
		bareKey := strings.TrimPrefix(instance.Key, node.ModuleInfo.Prefix())
		evalCtx.SetResource(bareKey, urn, markedOutputs)
	} else {
		evalCtx.SetResource(instance.Key, urn, markedOutputs)
	}

	return nil
}

// buildResourceOptionsInContext builds resource options using the provided eval context and parent URN.
func (e *Engine) buildResourceOptionsInContext(
	ctx context.Context, res *ast.Resource, instance *graph.ExpandedResource,
	evalCtx *eval.Context, parentURN urn.URN,
	modInfo *graph.ModuleInfo, modInst *moduleInstance, resourceMapping *bridge.BodyMapping,
	inputProps, outputProps []*schema.Property, inputs property.Map,
) (*ResourceOptions, error) {
	opts := &ResourceOptions{}
	opts.Parent = parentURN

	resPrefix := ""
	if modInfo != nil {
		resPrefix = modInfo.Prefix()
	}

	for _, dep := range res.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
			if urn := ctyAsString(outputs.GetAttr("urn")); urn != "" {
				opts.DependsOn = append(opts.DependsOn, urn)
			}
		}
	}

	// Handle lifecycle options
	if res.Lifecycle != nil {
		if res.Lifecycle.PreventDestroy != nil && *res.Lifecycle.PreventDestroy {
			opts.Protect = true
		}
		// ignore_changes maps to ignoreChanges. The traversal names are TF
		// (snake_case) attribute names; the Pulumi engine matches ignoreChanges
		// paths against Pulumi (camelCase) property names, so they must be
		// translated through the bridge mapping first.
		for _, ic := range res.Lifecycle.IgnoreChanges {
			icStr, err := translateAttrPathTraversal(ic, resourceMapping, inputProps)
			if err != nil {
				return nil, fmt.Errorf("invalid property path: %w", err)
			}
			if !ignoreChangesApplies(ic, resourceMapping, inputProps, inputs) {
				continue
			}
			opts.IgnoreChanges = append(opts.IgnoreChanges, icStr)
		}
		if res.Lifecycle.IgnoreAllChanges {
			opts.IgnoreChanges = []property.Glob{property.GlobFromSegments(property.Splat)}
		}
	}
	// create_before_destroy controls replacement order, with TF semantics:
	//   - true: create new, then delete old
	//   - false or absent: delete old, then create new (TF default)
	//
	// Mapped to Pulumi's inverse `deleteBeforeReplace`:
	//   - cbd=true  -> DeleteBeforeReplace=false
	//   - cbd=false -> DeleteBeforeReplace=true
	//   - cbd unset -> DeleteBeforeReplace=true (TF default, opposite of Pulumi's)
	//
	// create_before_destroy also propagates to a resource's dependencies, so a
	// dependency of a create-before-destroy resource is forced to the same
	// ordering even when it does not declare it (forcedCBD, computed from the
	// graph). This keeps every create in a replacement chain ahead of the deletes.
	cbd := res.Lifecycle != nil && res.Lifecycle.CreateBeforeDestroy != nil && *res.Lifecycle.CreateBeforeDestroy
	cbd = cbd || e.forcedCBD[instance.OriginalKey]
	opts.DeleteBeforeReplaceDef = true
	opts.DeleteBeforeReplace = !cbd

	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
				if parentURN := ctyAsString(outputs.GetAttr("urn")); parentURN != "" {
					opts.Parent = urn.URN(parentURN) // TODO: Don't look at attrs for this
				}
			}
		}
	}

	if res.Provider != nil {
		ref, err := e.resolveExplicitProvider(res.Provider, evalCtx, modInfo)
		if err != nil {
			return nil, fmt.Errorf("resolving provider for %s.%s: %w", res.Type, res.Name, err)
		}
		if ref != "" {
			opts.Provider = ref
		}
	} else if modInfo != nil {
		// Implicit default in a module: try a pass-through entry, then
		// the in-module `provider "<pkg>" {}` block (registered earlier
		// at key resPrefix+pkg in resourceOutputs), then an inherited
		// ancestor default. If none exist, fall through to Pulumi's
		// engine default.
		pkg := packageNameFromResourceType(res.Type)
		if ref := e.resolvePassThroughProvider(modInfo, pkg); ref != "" {
			opts.Provider = ref
		} else if outputs, ok := e.resourceOutputs.Get(resPrefix + pkg); ok {
			if ref, err := providerRefFromCty(outputs); err == nil {
				opts.Provider = ref
			}
		} else if ref := e.inheritedDefaultProvider(modInfo, pkg); ref != "" {
			opts.Provider = ref
		}
	} else {
		if ref, ok := e.defaultProviders.Get(packageNameFromResourceType(res.Type)); ok {
			opts.Provider = ref
		}
	}

	for _, traversal := range res.Providers {
		providerKey := graph.FormatTraversal(traversal)
		if providerKey == "" {
			continue
		}
		if providerOutputs, ok := e.resourceOutputs.Get(resPrefix + providerKey); ok {
			urn := ctyAsString(providerOutputs.GetAttr("urn"))
			id := ctyAsString(providerOutputs.GetAttr("id"))
			if urn != "" && id != "" {
				pkgName := packageNameFromResourceType(strings.SplitN(providerKey, ".", 2)[0])
				if opts.Providers == nil {
					opts.Providers = make(map[string]string)
				}
				opts.Providers[pkgName] = urn + "::" + id
			}
		}
	}

	// Handle timeouts
	if res.Timeouts != nil {
		ct := &CustomTimeouts{}
		hasTimeouts := false
		evalTimeout := func(expr hcl.Expression) (float64, bool) {
			if expr == nil {
				return 0, false
			}
			val, diags := e.evaluator.EvaluateExpression(expr)
			if diags.HasErrors() {
				return 0, false
			}
			val, _ = val.Unmark()
			if val.Type() != cty.String || val.IsNull() || !val.IsKnown() {
				return 0, false
			}
			d, err := time.ParseDuration(val.AsString())
			if err != nil {
				return 0, false
			}
			return d.Seconds(), true
		}
		if v, ok := evalTimeout(res.Timeouts.Create); ok {
			ct.Create = v
			hasTimeouts = true
		}
		if v, ok := evalTimeout(res.Timeouts.Read); ok {
			ct.Read = v
			hasTimeouts = true
		}
		if v, ok := evalTimeout(res.Timeouts.Update); ok {
			ct.Update = v
			hasTimeouts = true
		}
		if v, ok := evalTimeout(res.Timeouts.Delete); ok {
			ct.Delete = v
			hasTimeouts = true
		}
		if hasTimeouts {
			opts.CustomTimeouts = ct
		}
	}

	// Handle moved blocks - resolve aliases from moved blocks that target this resource
	movedAliases := e.resolveMovedAliases(ctx, res, instance.Index, instance.EachKeyString(), modInst)
	opts.Aliases = append(opts.Aliases, movedAliases...)

	// Handle aliases attribute
	if res.Aliases != nil {
		aliases, err := e.evaluateAliases(res.Aliases)
		if err == nil {
			opts.Aliases = append(opts.Aliases, aliases...)
		}
	}

	// Handle import blocks - resolve import ID from import blocks that target this resource
	importId, err := e.resolveImportId(res, instance.Index, instance.EachKeyString(), modInst)
	if err != nil {
		return nil, err
	}
	opts.ImportId = importId

	hclCtx := evalCtx.HCLContext()

	for _, t := range res.AdditionalSecretOutputs {
		name, err := translateSecretOutputName(t, resourceMapping, outputProps)
		if err != nil {
			return nil, err
		}
		opts.AdditionalSecretOutputs = append(opts.AdditionalSecretOutputs, name)
	}

	// Properties the schema declares as secret are marked as secret outputs, so
	// the engine stores and surfaces them as secrets just as the generated SDKs do.
	for _, p := range outputProps {
		if p.Secret && !slices.Contains(opts.AdditionalSecretOutputs, p.Name) {
			opts.AdditionalSecretOutputs = append(opts.AdditionalSecretOutputs, p.Name)
		}
	}

	if res.RetainOnDelete != nil {
		val, diags := res.RetainOnDelete.Value(hclCtx)
		val, _ = val.Unmark()
		if !diags.HasErrors() && val.Type() == cty.Bool && !val.IsNull() && val.IsKnown() {
			b := val.True()
			opts.RetainOnDelete = &b
		}
	}

	if res.DeletedWith != nil {
		depKey := graph.FormatTraversal(res.DeletedWith)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
				if urn := ctyAsString(outputs.GetAttr("urn")); urn != "" {
					opts.DeletedWith = urn
				}
			}
		}
	}

	for _, ref := range res.ReplaceWith {
		depKey := graph.FormatTraversal(ref)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
			if urn := ctyAsString(outputs.GetAttr("urn")); urn != "" {
				opts.ReplaceWith = append(opts.ReplaceWith, urn)
			}
		}
	}

	// hide_diffs and replace_on_changes name properties of this resource by
	// their attribute path. Like ignore_changes, the path may be written in TF
	// (snake_case) convention and must be translated to the Pulumi property
	// name the engine expects; a path already in Pulumi form passes through.
	for _, p := range res.HideDiff {
		glob, err := translateAttrPathTraversal(p, resourceMapping, inputProps)
		if err != nil {
			return nil, fmt.Errorf("invalid hide_diffs property path: %w", err)
		}
		opts.HideDiffs = append(opts.HideDiffs, glob)
	}
	for _, p := range res.ReplaceOnChanges {
		glob, err := translateAttrPathTraversal(p, resourceMapping, inputProps)
		if err != nil {
			return nil, fmt.Errorf("invalid replace_on_changes property path: %w", err)
		}
		opts.ReplaceOnChanges = append(opts.ReplaceOnChanges, glob)
	}

	// `lifecycle { replace_triggered_by = [a, b, ...] }` evaluates each
	// expression and feeds the result to RegisterResource as the
	// ReplacementTrigger property value: any element flipping triggers a
	// replacement. A single-element list is unwrapped to a scalar so the
	// trigger value round-trips with Pulumi's scalar `replacementTrigger`.
	// A reference in a trigger also establishes a dependency, as in TF.
	//
	// An element whose value is a whole resource is action-based, not
	// value-based: it must fire when the referenced resource is replaced even
	// if every attribute value is unchanged. The value trigger covers in-place
	// updates (an update implies the object value changed); the replacement
	// action is covered by also listing the referenced instances' URNs in
	// ReplaceWith.
	if res.Lifecycle != nil && len(res.Lifecycle.ReplaceTriggeredBy) > 0 {
		vals := make([]cty.Value, 0, len(res.Lifecycle.ReplaceTriggeredBy))
		for _, expr := range res.Lifecycle.ReplaceTriggeredBy {
			val, diags := expr.Value(hclCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating replace_triggered_by on %q: %s",
					res.Type+"."+res.Name, diags.Error())
			}
			for _, dep := range eval.CollectDepURNs(val) {
				if !slices.Contains(opts.DependsOn, dep) {
					opts.DependsOn = append(opts.DependsOn, dep)
				}
			}
			vals = append(vals, val)
			opts.ReplaceWith = append(opts.ReplaceWith, resourceURNsFromValue(val)...)
		}
		var triggerVal cty.Value
		if len(vals) == 1 {
			triggerVal = vals[0]
		} else {
			triggerVal = cty.TupleVal(vals)
		}
		pv, err := transform.CtyToPropertyValue(triggerVal)
		if err != nil {
			return nil, fmt.Errorf("converting replace_triggered_by on %q: %w",
				res.Type+"."+res.Name, err)
		}
		opts.ReplacementTrigger = pv
	}

	// Handle inline import_id attribute
	if res.ImportID != "" {
		opts.ImportId = res.ImportID
	}

	if res.EnvVarMappings != nil {
		val, diags := res.EnvVarMappings.Value(hclCtx)
		val, _ = val.UnmarkDeep()
		if !diags.HasErrors() && (val.Type().IsObjectType() || val.Type().IsMapType()) &&
			!val.IsNull() && val.IsKnown() {
			mappings := make(map[string]string)
			for k, v := range val.AsValueMap() {
				if v.Type() == cty.String && !v.IsNull() && v.IsKnown() {
					mappings[k] = v.AsString()
				}
			}
			if len(mappings) > 0 {
				opts.EnvVarMappings = mappings
			}
		}
	}

	if res.Version != nil {
		val, diags := res.Version.Value(hclCtx)
		val, _ = val.Unmark()
		if !diags.HasErrors() && val.Type() == cty.String && !val.IsNull() && val.IsKnown() {
			opts.Version = val.AsString()
		}
	}

	if res.PluginDownloadURL != nil && !strings.HasPrefix(res.Type, "pulumi_providers_") {
		val, diags := res.PluginDownloadURL.Value(hclCtx)
		val, _ = val.Unmark()
		if !diags.HasErrors() && val.Type() == cty.String && !val.IsNull() && val.IsKnown() {
			opts.PluginDownloadURL = val.AsString()
		}
	}

	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if parentOpts, ok := e.resourceInheritableOpts.Get(resPrefix + depKey); ok {
				if (res.Lifecycle == nil || res.Lifecycle.PreventDestroy == nil) &&
					parentOpts.Protect != nil && *parentOpts.Protect {
					opts.Protect = true
				}
				if res.RetainOnDelete == nil && parentOpts.RetainOnDelete != nil {
					opts.RetainOnDelete = parentOpts.RetainOnDelete
				}
			}
		}
	}

	return opts, nil
}

// targetAddr is a parsed `moved` or `import` block address: optional
// module-call steps followed by an optional resource. A whole-module-call
// address (e.g. `module.a`) has an empty Type.
type targetAddr struct {
	modules  []modulepath.Step // module-call steps, outermost first
	Type     string            // resource type, or "" for a whole-module-call address
	Name     string            // resource name
	keyIndex *int              // count instance key, if the address is keyed by count
	keyEach  *string           // for_each instance key, if the address is keyed by for_each
}

// keyed reports whether the address names a specific count/for_each instance.
func (a targetAddr) keyed() bool { return a.keyIndex != nil || a.keyEach != nil }

// parseTargetAddr decodes a `moved` or `import` address traversal into its
// module-call steps and (optional) resource. It returns false for a traversal
// it cannot model.
func parseTargetAddr(t hcl.Traversal) (targetAddr, bool) {
	var a targetAddr
	head := func(step hcl.Traverser) (string, bool) {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			return s.Name, true
		case hcl.TraverseAttr:
			return s.Name, true
		}
		return "", false
	}
	i := 0
	for i < len(t) {
		name, ok := head(t[i])
		if !ok || name != "module" {
			break
		}
		i++
		if i >= len(t) {
			return a, false
		}
		modName, ok := head(t[i])
		if !ok {
			return a, false
		}
		i++
		step := modulepath.NewStep(modName)
		if i < len(t) {
			if idx, ok := t[i].(hcl.TraverseIndex); ok {
				keyed, ok := moduleStepFor(modName, idx.Key)
				if !ok {
					return a, false
				}
				step = keyed
				i++
			}
		}
		a.modules = append(a.modules, step)
	}
	if i >= len(t) {
		// Whole-module-call address (no trailing resource).
		return a, len(a.modules) > 0
	}
	typ, ok := head(t[i])
	if !ok {
		return a, false
	}
	i++
	if i >= len(t) {
		return a, false
	}
	name, ok := head(t[i])
	if !ok {
		return a, false
	}
	i++
	a.Type, a.Name = typ, name
	if i < len(t) {
		if idx, ok := t[i].(hcl.TraverseIndex); ok {
			a.keyIndex, a.keyEach = instanceKeyFromCty(idx.Key)
			i++
		}
	}
	return a, i == len(t)
}

// instanceKeyFromCty decodes an instance-key value into its typed components: a
// count index for a number, a for_each key for a string. Both are nil for any
// other type, which the caller treats as an unkeyed address.
func instanceKeyFromCty(v cty.Value) (index *int, eachKey *string) {
	switch v.Type() {
	case cty.Number:
		iv, _ := v.AsBigFloat().Int64()
		n := int(iv)
		return &n, nil
	case cty.String:
		s := v.AsString()
		return nil, &s
	default:
		return nil, nil
	}
}

// moduleStepFor builds a module-call path step for `module.<name>[<key>]`,
// decoding the count index or for_each key. It returns false for a key that is
// neither a non-negative integer nor a string.
func moduleStepFor(name string, key cty.Value) (modulepath.Step, bool) {
	switch key.Type() {
	case cty.Number:
		iv, _ := key.AsBigFloat().Int64()
		if iv < 0 {
			return modulepath.Step{}, false
		}
		return modulepath.NewIndexedStep(name, int(iv)), true
	case cty.String:
		return modulepath.NewKeyedStep(name, key.AsString()), true
	default:
		return modulepath.Step{}, false
	}
}

// resolveMovedAliases finds the `moved` blocks that rename the resource instance
// being registered and returns the aliases recording its prior addresses, so a
// rename is treated as a move rather than a replacement.
//
// A moved block's addresses are relative to the module it is written in, so the
// resolver walks the resource's own module and every ancestor module. It handles
// resource renames within a module (including `count`/`for_each` instance-key
// changes), changes of the resource's type, moves of a resource between the
// root and a module or between two modules, and resources carried along when an
// enclosing module call is renamed or re-keyed (the matching component alias is
// attached in processModuleInit).
//
// Moved blocks chain: a prior address may itself be the `to` of another moved
// block (a -> b, then b -> c). Every address along the chain is aliased, so the
// engine matches whichever of them the state holds.
func (e *Engine) resolveMovedAliases(
	ctx context.Context, res *ast.Resource, index *int, eachKey *string, modInst *moduleInstance,
) []Alias {
	var aliases []Alias

	// resPath is the resource's module instance path (with any count/for_each
	// keys), which is what a resolved moved address is matched against.
	resPath := modulepath.Root()
	if modInst != nil {
		resPath = modInst.Path
	}

	// A resource address at some point along the chain of moves.
	type address struct {
		path    modulepath.Path
		typ     string
		name    string
		index   *int
		eachKey *string
	}
	type addrKey struct {
		typ    string
		prefix string
	}
	mkAddrKey := func(a address) addrKey {
		return addrKey{a.typ, prefixWithModulePath(a.path, buildResourceName(a.name, a.index, a.eachKey))}
	}

	work := []address{{path: resPath, typ: res.Type, name: res.Name, index: index, eachKey: eachKey}}
	seen := map[addrKey]bool{mkAddrKey(work[0]): true}

	for len(work) > 0 {
		cur := work[0]
		work = work[1:]
		for _, scope := range ancestorPaths(cur.path) {
			for _, moved := range e.graph.MovedBlocks(scope) {
				to, ok := parseTargetAddr(moved.To)
				if !ok || to.Type == "" { // skip whole-module-call moves
					continue
				}
				toPath := appendModuleSteps(scope, to.modules)
				if toPath != cur.path || to.Type != cur.typ || to.Name != cur.name {
					continue
				}
				from, ok := parseTargetAddr(moved.From)
				if !ok || from.Type == "" {
					continue
				}
				fromPath := appendModuleSteps(scope, from.modules)

				// Determine the prior instance key. A keyed endpoint on either side
				// makes this an instance move taking the prior key from `from` (an
				// unkeyed endpoint paired with a keyed one names the no-key
				// instance); with both endpoints unkeyed it is a whole-resource
				// rename that maps every instance to the same key.
				var priorIdx *int
				var priorEach *string
				switch {
				case to.keyed():
					if !instanceKeysEqual(cur.index, cur.eachKey, to.keyIndex, to.keyEach) {
						continue
					}
					priorIdx, priorEach = from.keyIndex, from.keyEach
				case from.keyed():
					if cur.index != nil || cur.eachKey != nil {
						continue
					}
					priorIdx, priorEach = from.keyIndex, from.keyEach
				default:
					priorIdx, priorEach = cur.index, cur.eachKey
				}

				prior := address{path: fromPath, typ: from.Type, name: from.Name, index: priorIdx, eachKey: priorEach}
				if seen[mkAddrKey(prior)] {
					continue
				}
				seen[mkAddrKey(prior)] = true
				work = append(work, prior)

				// The prior name is the resource's own name under its prior module
				// path; the prior parent is described relative to where it is now.
				name := prefixWithModulePath(fromPath, buildResourceName(from.Name, priorIdx, priorEach))
				parentURN, noParent, ok := e.priorParentSpec(fromPath, resPath, modInst)
				if !ok {
					continue
				}

				// A `moved` may also change the resource's type; the alias then
				// carries the prior type's token so the engine matches the old URN.
				var priorType string
				if from.Type != res.Type {
					priorRes, err := e.resolver.ResolveResource(ctx, from.Type)
					if err != nil {
						logging.V(5).Infof("moved: cannot resolve prior type %q: %v", from.Type, err)
						continue
					}
					priorType = priorRes.Token
				}

				aliases = append(aliases, Alias{Spec: &AliasSpec{
					Name:      name,
					Type:      priorType,
					ParentURN: parentURN,
					NoParent:  noParent,
				}})
			}
		}
	}

	// A `moved` block that renames an enclosing module call moves this resource
	// with it. The resource keeps its own name within the module, so it is
	// aliased to the name it had under each prior module path; Pulumi combines
	// that with the renamed component's own alias to recover the old URN.
	for _, oldPath := range e.oldModulePaths(resPath) {
		name := prefixWithModulePath(oldPath, buildResourceName(res.Name, index, eachKey))
		aliases = append(aliases, Alias{Spec: &AliasSpec{Name: name}})
	}

	return aliases
}

// oldModulePaths applies the whole-module-call `moved` blocks that rename a
// module enclosing (or equal to) path, following chained renames, and returns
// every prior module path the object lived at, nearest rename first. It
// returns nil when none applies.
func (e *Engine) oldModulePaths(path modulepath.Path) []modulepath.Path {
	var prior []modulepath.Path
	seen := map[modulepath.Path]bool{path: true}
	for {
		next, ok := e.priorModulePath(path)
		if !ok || seen[next] {
			return prior
		}
		seen[next] = true
		prior = append(prior, next)
		path = next
	}
}

// priorModulePath applies the first whole-module-call `moved` block that
// renames a module enclosing (or equal to) path, returning the module path the
// object lived at before that rename. ok is false when none applies.
func (e *Engine) priorModulePath(path modulepath.Path) (_ modulepath.Path, ok bool) {
	for _, scope := range ancestorPaths(path) {
		for _, moved := range e.graph.MovedBlocks(scope) {
			to, ok := parseTargetAddr(moved.To)
			if !ok || to.Type != "" || len(to.modules) == 0 {
				continue // not a whole-module-call address
			}
			from, ok := parseTargetAddr(moved.From)
			if !ok || from.Type != "" || len(from.modules) == 0 {
				continue
			}
			toPath := appendModuleSteps(scope, to.modules)
			suffix, ok := stripModulePrefix(path, toPath)
			if !ok {
				continue
			}
			fromPath := appendModuleSteps(scope, from.modules)
			for _, s := range suffix {
				fromPath = fromPath.Append(s)
			}
			return fromPath, true
		}
	}
	return path, false
}

// moduleComponentAliases returns the aliases for a module instance's component
// resource when `moved` blocks rename its call, so the component (and its
// children) are recognized as moved rather than replaced.
func (e *Engine) moduleComponentAliases(instPath modulepath.Path) []Alias {
	var aliases []Alias
	for _, oldPath := range e.oldModulePaths(instPath) {
		aliases = append(aliases, Alias{Spec: &AliasSpec{Name: oldPath.LogicalName()}})
	}
	return aliases
}

// priorParentSpec describes the parent of a resource at its prior moved address,
// relative to where it is registered now: the parent is unchanged within the
// same module, was the stack (NoParent) when the prior address was the root, or
// is a specific module component otherwise. ok is false when that prior
// component cannot be resolved.
func (e *Engine) priorParentSpec(
	fromPath, resPath modulepath.Path, modInst *moduleInstance,
) (parentURN string, noParent, ok bool) {
	switch {
	case fromPath == resPath:
		return "", false, true // parent unchanged; the alias inherits it
	case fromPath.IsRoot():
		return "", true, true // prior parent was the stack
	default:
		u, ok := e.priorComponentURN(fromPath, modInst)
		return string(u), false, ok
	}
}

// priorComponentURN returns the component URN of the module a resource moved out
// of. It uses the module's live component when that module still exists in the
// run; otherwise, when the resource now lives in a sibling module of the same
// source, it derives the URN from that sibling's component (same component type).
func (e *Engine) priorComponentURN(
	fromPath modulepath.Path, modInst *moduleInstance,
) (urn.URN, bool) {
	if insts, ok := e.moduleInstances.Get(fromPath); ok && len(insts) > 0 {
		return insts[0].URN, true
	}
	if modInst != nil {
		// Same component type as the resource's own module, so renaming the
		// resource's component URN to the prior module's name yields it.
		return modInst.URN.Rename(fromPath.LogicalName()), true
	}
	return "", false
}

// pathSteps returns p's steps, root first.
func pathSteps(p modulepath.Path) []modulepath.Step {
	var steps []modulepath.Step
	p.Steps(func(s modulepath.Step) bool {
		steps = append(steps, s)
		return true
	})
	return steps
}

// stripModulePrefix returns the steps of path that follow prefix, and whether
// prefix is a prefix of path.
func stripModulePrefix(path, prefix modulepath.Path) ([]modulepath.Step, bool) {
	ps, pre := pathSteps(path), pathSteps(prefix)
	if len(pre) > len(ps) {
		return nil, false
	}
	for i := range pre {
		if ps[i] != pre[i] {
			return nil, false
		}
	}
	return ps[len(pre):], true
}

// ancestorPaths returns p and all of its ancestors, root first.
func ancestorPaths(p modulepath.Path) []modulepath.Path {
	var chain []modulepath.Path
	for {
		chain = append(chain, p)
		parent, _, ok := p.Parent()
		if !ok {
			break
		}
		p = parent
	}
	slices.Reverse(chain)
	return chain
}

// appendModuleSteps extends base by the module-call steps of a moved address.
func appendModuleSteps(base modulepath.Path, steps []modulepath.Step) modulepath.Path {
	for _, s := range steps {
		base = base.Append(s)
	}
	return base
}

// instanceKeysEqual reports whether two resource instance keys name the same
// instance. Keys of different kinds (count vs for_each) never match, and two
// unkeyed (single-instance) addresses do not either.
func instanceKeysEqual(aIdx *int, aEach *string, bIdx *int, bEach *string) bool {
	switch {
	case aIdx != nil && bIdx != nil:
		return *aIdx == *bIdx
	case aEach != nil && bEach != nil:
		return *aEach == *bEach
	default:
		return false
	}
}

// resolveImportId finds the import block that targets this resource instance
// and returns its import ID. An import address names a single instance: its
// module-call steps must match the resource's module instance path, and on a
// count/for_each resource each instance takes only the ID of the import block
// keyed with its own instance key.
func (e *Engine) resolveImportId(
	res *ast.Resource, index *int, eachKey *string, modInst *moduleInstance,
) (string, error) {
	resPath := modulepath.Root()
	if modInst != nil {
		resPath = modInst.Path
	}

	for _, imp := range e.config.Imports {
		to, ok := parseTargetAddr(imp.To)
		if !ok || to.Type != res.Type || to.Name != res.Name {
			continue
		}
		if appendModuleSteps(modulepath.Root(), to.modules) != resPath {
			continue
		}
		if to.keyed() || index != nil || eachKey != nil {
			if !instanceKeysEqual(index, eachKey, to.keyIndex, to.keyEach) {
				continue
			}
		}
		return e.evaluateImportId(imp)
	}

	return "", nil
}

// evaluateImportId evaluates an import block's id expression, which must
// produce a known, non-null, non-sensitive string at plan time.
func (e *Engine) evaluateImportId(imp *ast.Import) (string, error) {
	errf := func(detail string) error {
		var subject *hcl.Range
		if imp.Id != nil {
			subject = imp.Id.Range().Ptr()
		} else {
			subject = imp.DeclRange.Ptr()
		}
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid import id argument",
			Detail:   detail,
			Subject:  subject,
		}}
	}

	if imp.Id == nil {
		return "", errf("The import ID cannot be null.")
	}
	val, diags := e.evaluator.EvaluateExpression(imp.Id)
	if diags.HasErrors() {
		return "", diags
	}
	sensitive := val.HasMark(eval.SensitiveMark)
	val, _ = val.Unmark()
	if val.IsNull() {
		return "", errf("The import ID cannot be null.")
	}
	if !val.IsKnown() {
		return "", errf(`The import block "id" argument depends on resource attributes that cannot be determined until apply.`)
	}
	if sensitive {
		return "", errf("The import ID cannot be sensitive.")
	}
	converted, err := ctyconvert.Convert(val, cty.String)
	if err != nil {
		return "", errf(fmt.Sprintf("The import ID value is unsuitable: %s.", err))
	}
	return converted.AsString(), nil
}

// evaluateAliases evaluates the aliases expression and returns a list of Alias values.
// Each alias can be a URN string or an object with spec fields.
func (e *Engine) evaluateAliases(expr hcl.Expression) ([]Alias, error) {
	val, diags := expr.Value(e.evaluator.Context().HCLContext())
	if diags.HasErrors() {
		return nil, diags
	}
	if !val.Type().IsListType() && !val.Type().IsTupleType() {
		return nil, fmt.Errorf("aliases must be a list")
	}
	var aliases []Alias
	it := val.ElementIterator()
	for it.Next() {
		_, elem := it.Element()
		if elem.Type() == cty.String {
			aliases = append(aliases, Alias{URN: ctyAsString(elem)})
		} else if elem.Type().IsObjectType() {
			spec := &AliasSpec{}
			objType := elem.Type()
			if objType.HasAttribute("name") {
				spec.Name = ctyAsString(elem.GetAttr("name"))
			}
			if objType.HasAttribute("type") {
				spec.Type = ctyAsString(elem.GetAttr("type"))
			}
			if objType.HasAttribute("stack") {
				spec.Stack = ctyAsString(elem.GetAttr("stack"))
			}
			if objType.HasAttribute("project") {
				spec.Project = ctyAsString(elem.GetAttr("project"))
			}
			if objType.HasAttribute("parent_urn") {
				spec.ParentURN = ctyAsString(elem.GetAttr("parent_urn"))
			}
			if objType.HasAttribute("no_parent") {
				if v, _ := elem.GetAttr("no_parent").Unmark(); v.Type() == cty.Bool && !v.IsNull() && v.IsKnown() {
					spec.NoParent = v.True()
				}
			}
			aliases = append(aliases, Alias{Spec: spec})
		}
	}
	return aliases, nil
}

// packageNameFromResourceType extracts the provider package name from an HCL resource type.
// For example, "config_resource" returns "config" and "pulumi_providers_config" returns "config".
func packageNameFromResourceType(token string) string {
	if name, ok := strings.CutPrefix(token, "pulumi_providers_"); ok {
		return name
	}
	return strings.SplitN(token, "_", 2)[0]
}

// packageRefForType returns the RegisterPackage ref for the given HCL resource type, or empty if none.
func (e *Engine) packageRefForType(hclToken string) PackageRef {
	return e.packageRefs[packageNameFromResourceType(hclToken)]
}

// packageRefForResource returns the registered package ref for a resource. An
// extension resource's token lives in the base provider's namespace, but its
// resolved schema names the extension package (e.g. "myext"), which is the one
// registered as a parameterized package — using it lets the engine record the
// resource's ExtensionRef.
func (e *Engine) packageRefForResource(hclToken string, resSchema *schema.Resource) PackageRef {
	if resSchema != nil && resSchema.PackageReference != nil {
		if ref, ok := e.packageRefs[resSchema.PackageReference.Name()]; ok {
			return ref
		}
	}
	return e.packageRefForType(hclToken)
}

// providerPackageName maps a provider's required_providers local name to its
// Pulumi package name: the basename of the entry's source ("hashicorp/simple"
// → "simple"), or the local name itself when no entry renames it.
func providerPackageName(tfBlock *ast.Terraform, local string) string {
	if tfBlock != nil {
		if req, ok := tfBlock.RequiredProviders[local]; ok && req.Source != "" {
			parts := strings.Split(req.Source, "/")
			return parts[len(parts)-1]
		}
	}
	return local
}

func (e *Engine) providerPackageName(local string) string {
	return providerPackageName(e.config.Terraform, local)
}

func knownProviders(tfBlock *ast.Terraform) []string {
	if tfBlock == nil {
		return nil
	}
	providers := make([]string, 0, len(tfBlock.RequiredProviders))
	for name := range tfBlock.RequiredProviders {
		providers = append(providers, name)
	}
	return providers
}

// hasFailedDependency reports whether any dependency of res is in failedNodes.
// When true, the resource should be skipped so that only genuinely independent
// resources are registered with the engine.
func (e *Engine) hasFailedDependency(res *ast.Resource) bool {
	// Check explicit depends_on traversals.
	for _, dep := range res.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey != "" {
			if _, failed := e.failedNodes.Get(depKey); failed {
				return true
			}
		}
	}
	// Check resource body expressions. A dependency reached only through a
	// recover(value, recovery) value argument does not gate the resource: if it
	// failed, recover() supplies the recovery value instead.
	if res.Config != nil {
		attrs, _ := res.Config.JustAttributes()
		for _, attr := range attrs {
			for _, depKey := range eval.NonRecoverableDependencies(attr.Expr) {
				if _, failed := e.failedNodes.Get(depKey); failed {
					return true
				}
			}
		}
	}
	return false
}

// buildResourceName builds the Pulumi resource name from the logical name and
// instance key, using Terraform-style instance addressing: single instances
// use the logical name as-is, count instances get a "[N]" suffix, and ForEach
// instances get a `["key"]` suffix (see [modulepath.Step.LogicalName]).
func buildResourceName(logicalName string, index *int, eachKey *string) string {
	switch {
	case index != nil:
		return modulepath.NewIndexedStep(logicalName, *index).LogicalName()
	case eachKey != nil:
		return modulepath.NewKeyedStep(logicalName, *eachKey).LogicalName()
	default:
		return logicalName
	}
}

// prefixWithModulePath prefixes name with the logical name of the module
// instance path it lives in, joined with ".". The "." separator cannot appear
// in an HCL label, so the module prefix cannot collide with a dash in a
// sibling's label or for_each key. Used where only a path (not a resolved
// [moduleInstance]) is available — the moved-block alias reconstruction; live
// instances prefix with [joinModuleName] on the instance's resolved Name.
func prefixWithModulePath(path modulepath.Path, name string) string {
	if path.IsRoot() {
		return name
	}
	return joinModuleName(path.LogicalName(), name)
}

// joinModuleName joins an enclosing module instance's resolved name and a
// child name with ".". An empty parent (the root) leaves the name bare.
func joinModuleName(parentName, name string) string {
	if parentName == "" {
		return name
	}
	return parentName + "." + name
}

// evaluatePulumiName evaluates a `pulumi { name = ... }` override expression.
// ok is false when the expression evaluates to null, which means "no
// override"; subject names the block for error messages.
func evaluatePulumiName(expr hcl.Expression, hclCtx *hcl.EvalContext, subject string) (name string, ok bool, err error) {
	val, diags := expr.Value(hclCtx)
	if diags.HasErrors() {
		return "", false, fmt.Errorf("evaluating pulumi name for %s: %s", subject, diags.Error())
	}
	val, _ = val.Unmark()
	if val.IsNull() {
		return "", false, nil
	}
	if !val.IsKnown() || val.Type() != cty.String {
		return "", false, fmt.Errorf("pulumi name for %s must be a known string", subject)
	}
	return val.AsString(), true, nil
}

// resourceInstanceName computes the Pulumi resource name for one resource
// instance. A `pulumi { name = ... }` override is evaluated per instance
// (count.index/each.key and pulumi.module.name are in scope) and is the full
// logical name — no module prefix is applied; a null override falls back to
// the derived name. Derived names are prefixed with the enclosing module
// instance's resolved name (e.g. "many" or "many[0]") joined with ".".
func (*Engine) resourceInstanceName(
	res *ast.Resource, instance *graph.ExpandedResource, hclCtx *hcl.EvalContext, modInst *moduleInstance,
) (string, error) {
	if res.PulumiName != nil {
		name, ok, err := evaluatePulumiName(res.PulumiName, hclCtx, res.Type+"."+res.Name)
		if err != nil {
			return "", err
		}
		if ok {
			return name, nil
		}
	}
	name := buildResourceName(res.Name, instance.Index, instance.EachKeyString())
	if modInst == nil {
		return name, nil
	}
	return joinModuleName(modInst.Name, name), nil
}

// translateAttrPathTraversal formats an attribute-path traversal (e.g. an
// ignore_changes entry) into the dotted/bracketed path the Pulumi engine
// expects, translating each TF (snake_case) attribute segment to its Pulumi
// (camelCase) name.
func translateAttrPathTraversal(
	traversal hcl.Traversal, mapping *bridge.BodyMapping, props []*schema.Property,
) (property.Glob, error) {
	if len(traversal) == 0 {
		return property.Glob{}, nil
	}
	segments := make([]property.GlobSegment, 0, len(traversal))
	resolver := attrPathNameResolver{mapping: mapping, props: props}
	prevSingularBlock := false
	for _, step := range traversal {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			name, singularBlock, err := resolver.next(s.Name)
			if err != nil {
				return property.Glob{}, err
			}
			segments = append(segments, property.NewSegment(name))
			prevSingularBlock = singularBlock
		case hcl.TraverseAttr:
			name, singularBlock, err := resolver.next(s.Name)
			if err != nil {
				return property.Glob{}, err
			}
			segments = append(segments, property.NewSegment(name))
			prevSingularBlock = singularBlock
		case hcl.TraverseIndex:
			// The bridge flattens a MaxItems=1 block to a single object, so the
			// TF list index addressing it (settings[0]) has no Pulumi
			// counterpart: the flattened object property already stands in for
			// the sole element, and the index is dropped so the translated path
			// (settings.mode) matches.
			if prevSingularBlock {
				prevSingularBlock = false
				continue
			}
			// Index segments are dynamic map keys or list indices, not schema
			// properties: they are emitted verbatim and not validated (the
			// preceding attribute already advanced the resolver past the
			// collection), matching OpenTofu, which accepts foo["bar"] without
			// checking the key.
			key := s.Key
			if key.Type() == cty.String {
				segments = append(segments, property.NewSegment(key.AsString()))
			} else if key.Type() == cty.Number {
				i64, acc := key.AsBigFloat().Int64()
				if acc != big.Exact || i64 > math.MaxInt {
					return property.Glob{}, fmt.Errorf("unrepresentable path segment %s", key.AsBigFloat())
				}
				segments = append(segments, property.NewSegment(int(i64)))
			}
		}
	}

	return property.GlobFromSegments(segments...), nil
}

// ignoreChangesApplies reports whether an ignore_changes path resolves inside
// the evaluated inputs. OpenTofu silently skips an entry whose path does not
// apply to the configuration — removing a whole block un-ignores the
// attributes inside it, so e.g. a ForceNew change there is planned as a
// replacement again. Passing such an entry to the engine anyway would keep
// suppressing the diff, because the engine ignores by state paths too.
//
// Mirroring OpenTofu's check: a trailing map key is exempt (a key may be
// ignored while being added or removed), a missing leaf attribute still
// resolves (it decodes as null in OpenTofu's config), and an unknown value
// cannot be disproven, so those keep the entry. OpenTofu also requires the
// path to resolve in the prior state, which is not visible here.
func ignoreChangesApplies(
	traversal hcl.Traversal, mapping *bridge.BodyMapping, props []*schema.Property, inputs property.Map,
) bool {
	if len(traversal) > 1 {
		if idx, ok := traversal[len(traversal)-1].(hcl.TraverseIndex); ok && idx.Key.Type() == cty.String {
			traversal = traversal[:len(traversal)-1]
		}
	}
	resolver := attrPathNameResolver{mapping: mapping, props: props}
	v := property.New(inputs)
	prevSingularBlock := false
	for i, step := range traversal {
		if v.IsComputed() {
			return true
		}
		var tfName string
		switch s := step.(type) {
		case hcl.TraverseRoot:
			tfName = s.Name
		case hcl.TraverseAttr:
			tfName = s.Name
		case hcl.TraverseIndex:
			if prevSingularBlock {
				prevSingularBlock = false
				continue
			}
			switch {
			case s.Key.Type() == cty.Number && v.IsArray():
				idx, acc := s.Key.AsBigFloat().Int64()
				if acc != big.Exact || idx < 0 || idx >= int64(v.AsArray().Len()) {
					return false
				}
				v = v.AsArray().Get(int(idx))
			case s.Key.Type() == cty.String && v.IsMap():
				el, ok := v.AsMap().GetOk(s.Key.AsString())
				if !ok {
					return false
				}
				v = el
			default:
				return false
			}
			continue
		}
		name, singularBlock, err := resolver.next(tfName)
		if err != nil {
			return false
		}
		if !v.IsMap() {
			return false
		}
		el, ok := v.AsMap().GetOk(name)
		if !ok {
			return i == len(traversal)-1
		}
		v = el
		prevSingularBlock = singularBlock
	}
	return true
}

// ctyPathTraversal converts a cty.Path over an evaluated inputs object into an
// attribute-path traversal, so it can be translated to engine form by
// translateAttrPathTraversal like a hide_diffs entry.
func ctyPathTraversal(p cty.Path) hcl.Traversal {
	t := make(hcl.Traversal, 0, len(p))
	for _, step := range p {
		switch s := step.(type) {
		case cty.GetAttrStep:
			t = append(t, hcl.TraverseAttr{Name: s.Name})
		case cty.IndexStep:
			t = append(t, hcl.TraverseIndex{Key: s.Key})
		}
	}
	return t
}

// translateSecretOutputName translates a single additional_secret_outputs entry
// from its TF (snake_case) name to its Pulumi name. Unlike hide_diffs and
// replace_on_changes, an additional_secret_outputs entry names a single
// top-level output property rather than a nested path, so a multi-segment
// traversal is rejected.
func translateSecretOutputName(
	t hcl.Traversal, mapping *bridge.BodyMapping, props []*schema.Property,
) (string, error) {
	if len(t) != 1 {
		return "", fmt.Errorf(
			"invalid additional_secret_outputs entry %#v: expected a single top-level property name",
			formatAttrTraversal(t))
	}
	var name string
	switch s := t[0].(type) {
	case hcl.TraverseRoot:
		name = s.Name
	case hcl.TraverseAttr:
		name = s.Name
	default:
		return "", fmt.Errorf("invalid additional_secret_outputs entry: expected a property name")
	}
	resolver := attrPathNameResolver{mapping: mapping, props: props}
	pulumiName, _, err := resolver.next(name)
	return pulumiName, err
}

// formatAttrTraversal renders an attribute-path traversal as a dotted/bracketed
// string for diagnostics.
func formatAttrTraversal(t hcl.Traversal) string {
	var b strings.Builder
	for i, step := range t {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			b.WriteString(s.Name)
		case hcl.TraverseAttr:
			if i > 0 {
				b.WriteByte('.')
			}
			b.WriteString(s.Name)
		case hcl.TraverseIndex:
			if s.Key.Type() == cty.String {
				fmt.Fprintf(&b, "[%q]", s.Key.AsString())
			} else if s.Key.Type() == cty.Number {
				if i64, acc := s.Key.AsBigFloat().Int64(); acc == big.Exact {
					fmt.Fprintf(&b, "[%d]", i64)
				}
			}
		}
	}
	return b.String()
}

// attrPathNameResolver walks an attribute path, translating each attribute
// segment from its TF name to its Pulumi name and descending into the nested
// schema for the next segment.
type attrPathNameResolver struct {
	mapping *bridge.BodyMapping
	props   []*schema.Property
}

// next translates one TF (snake_case) attribute-name segment to its Pulumi name
// and advances the resolver into the nested schema for the following segment. It
// reports whether the resolved field is a MaxItems=1 block flattened to a single
// Pulumi object, so the caller can drop the TF list index that follows it.
func (r *attrPathNameResolver) next(tfName string) (name string, singularBlock bool, err error) {
	if fm := r.mapping.Lookup(tfName); fm != nil {
		if fm.Nested != nil {
			r.mapping, r.props = fm.Nested, nil
		} else {
			r.mapping, r.props = nil, nil
		}
		return fm.PulumiName, fm.TFBlock && fm.MaxItemsOne, nil
	}
	pulumiName, prop := transform.PulumiCaseFromSnakeCase(tfName, r.props)
	if prop != nil {
		r.mapping, r.props = nil, objectProperties(prop.Type)
		_, isObject := codegen.UnwrapType(prop.Type).(*schema.ObjectType)
		return pulumiName, isObject, nil
	}
	if r.mapping != nil || len(r.props) > 0 {
		return "", false, fmt.Errorf("unknown property %q", tfName)
	}
	r.mapping, r.props = nil, nil
	return tfName, false, nil
}

// objectProperties returns the nested properties of an object-typed schema,
// unwrapping array/map element types and optional wrappers, or nil when the
// type has no named properties.
func objectProperties(t schema.Type) []*schema.Property {
	switch tt := t.(type) {
	case *schema.ObjectType:
		return tt.Properties
	case *schema.ArrayType:
		return objectProperties(tt.ElementType)
	case *schema.MapType:
		return objectProperties(tt.ElementType)
	case *schema.OptionalType:
		return objectProperties(tt.ElementType)
	default:
		return nil
	}
}

// ResourceOptions contains resource registration options.
type ResourceOptions struct {
	Custom                  bool
	Remote                  bool
	DependsOn               []string
	PropertyDependencies    map[string][]string
	Protect                 bool
	IgnoreChanges           []property.Glob
	Aliases                 []Alias
	Provider                string
	Providers               map[string]string // Map from package name to provider reference (urn::id)
	Parent                  urn.URN
	DeleteBeforeReplace     bool
	DeleteBeforeReplaceDef  bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts          *CustomTimeouts
	ImportId                string
	AdditionalSecretOutputs []string
	RetainOnDelete          *bool
	DeletedWith             string          // URN of the resource that, when deleted, causes this resource to be deleted
	ReplaceWith             []string        // URNs of resources whose replacement triggers replacement of this resource
	HideDiffs               []property.Glob // Property paths whose diffs should not be displayed
	ReplaceOnChanges        []property.Glob // Property paths that if changed should force a replacement
	ReplacementTrigger      property.Value  // Value whose change triggers replacement
	EnvVarMappings          map[string]string
	Version                 string
	PluginDownloadURL       string
	PackageRef              PackageRef
	Hooks                   *ResourceHookBinding
}

// registerResource registers a resource with the Pulumi engine. tfType is the
// HCL resource type, used only to detect builtins (like terraform_data) that are
// lowered onto a different engine resource at this schemaless boundary.
func (e *Engine) registerResource(
	ctx context.Context,
	tfType string,
	typeToken string,
	name string,
	inputs property.Map,
	opts *ResourceOptions,
) (urn.URN, string, property.Map, error) {
	inputs = lowerTerraformDataInputs(tfType, inputs, opts)

	if typeToken == stackReferenceType {
		return e.readResource(ctx, typeToken, name, inputs, opts)
	}

	// Register with the resource monitor
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:                    typeToken,
		Name:                    name,
		Inputs:                  inputs,
		Dependencies:            opts.DependsOn,
		PropertyDependencies:    opts.PropertyDependencies,
		Custom:                  opts.Custom,
		Remote:                  opts.Remote,
		Protect:                 opts.Protect,
		IgnoreChanges:           opts.IgnoreChanges,
		Aliases:                 opts.Aliases,
		Provider:                opts.Provider,
		Providers:               opts.Providers,
		Parent:                  opts.Parent,
		DeleteBeforeReplace:     opts.DeleteBeforeReplace,
		DeleteBeforeReplaceDef:  opts.DeleteBeforeReplaceDef,
		CustomTimeouts:          opts.CustomTimeouts,
		ImportId:                opts.ImportId,
		AdditionalSecretOutputs: opts.AdditionalSecretOutputs,
		RetainOnDelete:          opts.RetainOnDelete,
		DeletedWith:             opts.DeletedWith,
		ReplaceWith:             opts.ReplaceWith,
		HideDiffs:               opts.HideDiffs,
		ReplaceOnChanges:        opts.ReplaceOnChanges,
		ReplacementTrigger:      opts.ReplacementTrigger,
		EnvVarMappings:          opts.EnvVarMappings,
		Version:                 opts.Version,
		PluginDownloadURL:       opts.PluginDownloadURL,
		PackageRef:              opts.PackageRef,
		Hooks:                   opts.Hooks,
	})
	if err != nil {
		return "", "", property.Map{}, err
	}

	return resp.URN, resp.ID, lowerTerraformDataOutputs(tfType, resp.Outputs, opts), nil
}

// stackReferenceType is the builtin resource the engine resolves against the
// backend. It is registered with a Read rather than a Create: the modern Pulumi
// SDKs read stack references (Go's ReadResource, Node's id-bearing
// CustomResource), and Create is deprecated for this type.
const stackReferenceType = "pulumi:pulumi:StackReference"

// readResource registers a resource via a Read. The ID is taken from the "name"
// input (the fully-qualified stack name), matching how the SDKs identify a
// stack reference.
func (e *Engine) readResource(
	ctx context.Context,
	typeToken string,
	name string,
	inputs property.Map,
	opts *ResourceOptions,
) (urn.URN, string, property.Map, error) {
	nameVal, ok := inputs.GetOk("name")
	if !ok || !nameVal.IsString() {
		return "", "", property.Map{}, fmt.Errorf("%s %q requires a string \"name\" input", typeToken, name)
	}

	resp, err := e.resmon.ReadResource(ctx, ReadResourceRequest{
		Type:                    typeToken,
		Name:                    name,
		ID:                      nameVal.AsString(),
		Inputs:                  inputs,
		Parent:                  opts.Parent,
		Dependencies:            opts.DependsOn,
		Provider:                opts.Provider,
		Version:                 opts.Version,
		AdditionalSecretOutputs: opts.AdditionalSecretOutputs,
		PluginDownloadURL:       opts.PluginDownloadURL,
		PackageRef:              opts.PackageRef,
	})
	if err != nil {
		return "", "", property.Map{}, err
	}

	return resp.URN, resp.ID, resp.Outputs, nil
}

// processDataSource processes a data source definition.
func (e *Engine) processDataSource(ctx context.Context, node *graph.Node) error {
	ds := node.Resource
	if ds == nil {
		return fmt.Errorf("data source node missing Resource field")
	}

	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			return e.processDataSourceInContext(ctx, node, ds, inst.EvalCtx)
		})
	}

	return e.processDataSourceInContext(ctx, node, ds, e.evaluator.Context())
}

func (e *Engine) processDataSourceInContext(
	ctx context.Context, node *graph.Node, ds *ast.Resource, evalCtx *eval.Context,
) error {
	funcSchema, err := e.resolver.ResolveFunction(ctx, ds.Type)
	if err != nil {
		if diag := unknownTokenDiag("data source", ds.TypeRange, err); diag != err {
			return diag
		}
		return fmt.Errorf("resolving data source type %s: %w", ds.Type, err)
	}

	dsKey := node.Key
	if node.ModuleInfo != nil {
		dsKey = strings.TrimPrefix(dsKey, node.ModuleInfo.Prefix())
	}
	dsKey = strings.TrimPrefix(dsKey, "data.")

	if ds.Count != nil || ds.ForEach != nil {
		return e.processRangedDataSource(ctx, node, ds, funcSchema, evalCtx, dsKey)
	}

	ctyOutputs, err := e.invokeDataSourceOnce(ctx, node, ds, funcSchema, evalCtx)
	if err != nil {
		return err
	}
	evalCtx.SetDataSource(dsKey, ctyOutputs)
	return nil
}

// processRangedDataSource handles a data source with count or for_each.
// It expands into N instances, invokes each, and stores the aggregated
// outputs as a tuple (count) or object keyed by each.key (for_each) so that
// `data.<type>.<name>[<index>|<key>].<attr>` resolves as expected.
func (e *Engine) processRangedDataSource(
	ctx context.Context, node *graph.Node, ds *ast.Resource, funcSchema *schema.Function,
	evalCtx *eval.Context, dsKey string,
) error {
	tempEvaluator := eval.NewEvaluator(evalCtx)
	expander := graph.NewResourceExpander()

	unknownArg := ""
	if ds.Count != nil {
		count, isBool, unknown, _, diags := tempEvaluator.EvaluateCount(ds.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating count: %s", diags.Error())
		}
		switch {
		case unknown:
			unknownArg = "count"
		case isBool:
			expander.SetBoolCount(node.Key, count)
		default:
			expander.SetCount(node.Key, count)
		}
	}

	if ds.ForEach != nil {
		forEach, unknown, _, diags := tempEvaluator.EvaluateForEach(ds.ForEach)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating for_each: %s", diags.Error())
		}
		if unknown {
			unknownArg = "for_each"
		} else {
			expander.SetForEach(node.Key, forEach)
		}
	}

	// See the matching case in processResourceInContext: unexpandable during
	// preview means no invokes and an unknown aggregate value.
	if unknownArg != "" {
		if !e.dryRun {
			return fmt.Errorf("%s: the %s value depends on values that are not yet known", node.Key, unknownArg)
		}
		evalCtx.SetDataSource(dsKey, cty.UnknownVal(cty.DynamicPseudoType))
		return nil
	}

	result := expander.Expand(node)

	var tupleOutputs []cty.Value
	eachOutputs := make(map[string]cty.Value)
	isForEach := ds.ForEach != nil

	for _, instance := range result.Instances {
		instCtx := evalCtx.WithIteration(instance.Index, instance.EachKey, instance.EachValue)

		ctyOut, invokeErr := e.invokeDataSourceOnce(ctx, node, ds, funcSchema, instCtx)

		if invokeErr != nil {
			return invokeErr
		}
		if isForEach {
			eachOutputs[instance.EachKey.AsString()] = ctyOut
		} else {
			tupleOutputs = append(tupleOutputs, ctyOut)
		}
	}

	var aggregated cty.Value
	if isForEach {
		if len(eachOutputs) == 0 {
			aggregated = cty.EmptyObjectVal
		} else {
			aggregated = cty.ObjectVal(eachOutputs)
		}
	} else {
		if len(tupleOutputs) == 0 {
			aggregated = cty.EmptyTupleVal
		} else {
			aggregated = cty.TupleVal(tupleOutputs)
		}
	}

	evalCtx.SetDataSource(dsKey, aggregated)
	return nil
}

// invokeDataSourceOnce performs a single data-source invocation using the
// current state of evalCtx (which may have each/count set by the caller).
// The returned outputs are marked with the URN deps gathered from inputs
// and explicit depends_on, so downstream reads of data.X.Y carry them
// without a separate dependency map.
func (e *Engine) invokeDataSourceOnce(
	ctx context.Context, node *graph.Node, ds *ast.Resource, funcSchema *schema.Function,
	evalCtx *eval.Context,
) (cty.Value, error) {
	hclCtx := evalCtx.HCLContext()

	// Failed preconditions prevent the read; unknown conditions defer.
	for i, rule := range ds.Preconditions {
		if err := evaluatePrecondition(rule, hclCtx, i+1, node.Key); err != nil {
			return cty.NilVal, err
		}
	}

	depMarks := cty.ValueMarks{}
	addURN := func(urn string) {
		if urn == "" {
			return
		}
		depMarks[eval.DepMark(urn)] = struct{}{}
	}

	// Check-rule references establish dependencies, as in TF; carry them on
	// the outputs alongside the input-derived deps so downstream readers
	// inherit them.
	for _, urn := range checkRuleDeps(ds.Preconditions, hclCtx) {
		addURN(urn)
	}
	for _, urn := range checkRuleDeps(ds.Postconditions, hclCtx) {
		addURN(urn)
	}

	dataSourceMapping := e.resolver.DataSourceBodyMapping(ctx, ds.Type)
	inputs, diags := transform.EvalFunctionWithSchema(ds.Config, funcSchema, dataSourceMapping,
		func(propKey resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
			var val cty.Value
			var diags hcl.Diagnostics
			if len(extraVars) > 0 {
				childCtx := hclCtx.NewChild()
				childCtx.Variables = extraVars
				val, diags = expr.Value(childCtx)
			} else {
				val, diags = expr.Value(hclCtx)
			}
			if diags.HasErrors() {
				return val, diags
			}

			for _, urn := range eval.CollectDepURNs(val) {
				addURN(urn)
			}

			return val, diags
		})
	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	invokeReq := InvokeRequest{
		Token:      funcSchema.Token,
		Args:       inputs,
		PackageRef: e.packageRefForType(ds.Type),
	}

	if ds.Provider != nil {
		ref, err := e.resolveExplicitProvider(ds.Provider, evalCtx, node.ModuleInfo)
		if err != nil {
			return cty.NilVal, fmt.Errorf("resolving provider for data %s.%s: %w", ds.Type, ds.Name, err)
		}
		if ref != "" {
			invokeReq.Provider = ref
		}
	} else if node.ModuleInfo != nil {
		pkg := packageNameFromResourceType(ds.Type)
		resPrefix := node.ModuleInfo.Prefix()
		if ref := e.resolvePassThroughProvider(node.ModuleInfo, pkg); ref != "" {
			invokeReq.Provider = ref
		} else if outputs, ok := e.resourceOutputs.Get(resPrefix + pkg); ok {
			if ref, err := providerRefFromCty(outputs); err == nil {
				invokeReq.Provider = ref
			}
		} else if ref := e.inheritedDefaultProvider(node.ModuleInfo, pkg); ref != "" {
			invokeReq.Provider = ref
		}
	} else {
		if ref, ok := e.defaultProviders.Get(packageNameFromResourceType(ds.Type)); ok {
			invokeReq.Provider = ref
		}
	}

	if ds.PluginDownloadURL != nil {
		val, valDiags := ds.PluginDownloadURL.Value(hclCtx)
		if !valDiags.HasErrors() && val.Type() == cty.String {
			invokeReq.PluginDownloadURL = ctyAsString(val)
		}
	}

	for _, dep := range ds.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey == "" {
			continue
		}
		fullDepKey := depKey
		if node.ModuleInfo != nil {
			fullDepKey = node.ModuleInfo.Prefix() + depKey
		}
		if outputs, ok := e.resourceOutputs.Get(fullDepKey); ok {
			addURN(ctyAsString(outputs.GetAttr("urn")))
		}
	}

	// Match the Node.js / Python SDK behavior: during preview, if any input
	// to the invoke is unknown, skip the provider call and synthesize an
	// all-unknown result.
	var outputs property.Map
	if !e.dryRun || !property.New(inputs).HasComputed() {
		var err error
		outputs, err = e.invokeFunction(ctx, ds.Type, invokeReq)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invoking data source: %w", err)
		}
	}

	ctyOutputs, err := transform.FunctionOutputToCty(outputs, funcSchema, dataSourceMapping, e.dryRun)
	if err != nil {
		return cty.NilVal, fmt.Errorf("converting function outputs to HCL types: %w", err)
	}

	for i, rule := range ds.Postconditions {
		if err := evaluatePostconditionValue(rule, hclCtx, ctyOutputs, i+1, node.Key); err != nil {
			return cty.NilVal, err
		}
	}

	return ctyOutputs.WithMarks(depMarks), nil
}

// processCall processes a call block (method invocation on a resource).
func (e *Engine) processCall(ctx context.Context, node *graph.Node) error {
	call := node.Call
	if call == nil {
		return fmt.Errorf("call node missing Call field")
	}

	// Find the resource or provider being called by logical name
	var resKey string
	var resType string
	var resSchema *schema.Resource
	var isProviderResource bool // true for resource "pulumi_providers_*" blocks

	for k, res := range e.config.Resources {
		if res.Name == call.ResourceName {
			resKey = k
			resType = res.Type
			var err error
			resSchema, err = e.resolver.ResolveResource(ctx, res.Type)
			if err != nil {
				if diag := unknownTokenDiag("resource", res.TypeRange, err); diag != err {
					return diag
				}
				return fmt.Errorf("resolving resource type %s for call: %w", res.Type, err)
			}
			isProviderResource = strings.HasPrefix(res.Type, "pulumi_providers_")
			break
		}
	}

	if resKey == "" {
		// Try providers — config.Providers is keyed by provider.Key(),
		// so we search by alias / name instead of by ResourceName directly.
		var matched *ast.Provider
		for _, p := range e.config.Providers {
			if p.Alias == call.ResourceName || (p.Alias == "" && p.Name == call.ResourceName) {
				matched = p
				break
			}
		}
		if matched != nil {
			resKey = matched.Key()
			providerToken := "pulumi_providers_" + e.providerPackageName(matched.Name)
			resType = providerToken
			pkg, err := packages.ResolvePackage(ctx, e.pkgLoader, knownProviders(e.config.Terraform), providerToken)
			if err != nil {
				return fmt.Errorf("resolving provider package for call: %w", err)
			}
			resSchema, err = pkg.Provider()
			if err != nil {
				return fmt.Errorf("resolving provider schema for call: %w", err)
			}
		}
	}

	if resKey == "" {
		return fmt.Errorf("call block references unknown resource or provider %q", call.ResourceName)
	}

	// Find the method in the resource schema by matching snake_case name
	var method *schema.Method
	for _, m := range resSchema.Methods {
		if transform.SnakeCaseFromPulumiCase(m.Name) == call.MethodName {
			method = m
			break
		}
	}
	if method == nil {
		return fmt.Errorf("resource %q has no method %q", call.ResourceName, call.MethodName)
	}

	// Look up resource outputs to get URN and ID
	outputs, ok := e.resourceOutputs.Get(resKey)
	if !ok {
		return fmt.Errorf("resource %q outputs not found", resKey)
	}

	urnStr := ctyAsString(outputs.GetAttr("urn"))
	if urnStr == "" {
		return fmt.Errorf("resource %q missing URN", resKey)
	}
	urn := resource.URN(urnStr)

	// Build __self__ resource reference
	var selfID property.Value
	if resSchema.IsComponent && !isProviderResource {
		selfID = property.New(property.Null)
	} else {
		idVal, _ := outputs.GetAttr("id").Unmark()
		if idVal.Type() == cty.String && idVal.IsKnown() && !idVal.IsNull() {
			selfID = property.New(idVal.AsString())
		} else if idVal.Type() == cty.String {
			selfID = property.New(property.Computed)
		} else {
			selfID = property.New(property.Null)
		}
	}
	selfRef := property.New(property.ResourceReference{
		URN: urn,
		ID:  selfID,
	})

	// Evaluate call arguments using the function schema, excluding __self__ which is
	// provided by the runtime (not the HCL body).
	filteredFunc := *method.Function
	if filteredFunc.Inputs != nil {
		filteredInputs := *filteredFunc.Inputs
		filteredInputs.Properties = slices.DeleteFunc(
			slices.Clone(filteredInputs.Properties),
			func(p *schema.Property) bool { return p.Name == "__self__" },
		)
		filteredFunc.Inputs = &filteredInputs
	}

	userArgs, diags := transform.EvalFunctionWithSchema(call.Config, &filteredFunc, nil,
		func(_ resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
			if len(extraVars) > 0 {
				childCtx := e.evaluator.Context().HCLContext().NewChild()
				childCtx.Variables = extraVars
				return expr.Value(childCtx)
			}
			return e.evaluator.EvaluateExpression(expr)
		})
	if diags.HasErrors() {
		return fmt.Errorf("evaluating call arguments for %s.%s: %s", call.ResourceName, call.MethodName, diags.Error())
	}

	ret, err := e.callMethod(ctx, CallRequest{
		Token:      method.Function.Token,
		Args:       userArgs.Set("__self__", selfRef),
		PackageRef: e.packageRefForType(resType),
	})
	if err != nil {
		return fmt.Errorf("calling method %s.%s: %w", call.ResourceName, call.MethodName, err)
	}

	// Convert return values to cty
	ctyOutputs, err := transform.FunctionOutputToCty(ret, method.Function, nil, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting call outputs to HCL types: %w", err)
	}

	// Store outputs keyed as "resourceName.methodName"
	callKey := ast.CallKey(call.ResourceName, call.MethodName)
	e.evaluator.Context().SetCall(callKey, ctyOutputs)

	return nil
}

// callMethod calls a method on a resource via the resource monitor.
func (e *Engine) callMethod(ctx context.Context, req CallRequest) (property.Map, error) {
	resp, err := e.resmon.Call(ctx, req)
	if err != nil {
		return property.Map{}, err
	}

	if len(resp.Failures) > 0 {
		return property.Map{}, fmt.Errorf("method call failed: %v", resp.Failures)
	}

	return resp.Return, nil
}

// installProviderFunctions builds the provider-defined function table for the
// module described by config and installs it on evalCtx, keyed as
// provider::<localname>::<name>. Only providers the parser saw function calls
// on are resolved, so provider schemas keep loading lazily. When the parser
// could not scan every file (JSON syntax), it falls back to (leniently)
// resolving every declared provider. modInfo is nil for the root module.
func (e *Engine) installProviderFunctions(
	ctx context.Context, evalCtx *eval.Context, config *ast.Config, modInfo *graph.ModuleInfo,
) error {
	table, err := e.providerFunctionTable(ctx, config, modInfo)
	if err != nil {
		return err
	}
	if len(table) > 0 {
		evalCtx.SetProviderFunctions(table)
	}
	return nil
}

// providerFunctionTable resolves and projects the provider-defined functions
// the module described by config can call. See installProviderFunctions.
func (e *Engine) providerFunctionTable(
	ctx context.Context, config *ast.Config, modInfo *graph.ModuleInfo,
) (map[string]function.Function, error) {
	referenced := map[string]struct{}{}
	for _, name := range config.ProviderFunctionCalls {
		referenced[name] = struct{}{}
	}
	lenient := false
	if config.ProviderFunctionCallsIncomplete && config.Terraform != nil {
		lenient = true
		for name := range config.Terraform.RequiredProviders {
			referenced[name] = struct{}{}
		}
	}
	if len(referenced) == 0 {
		return nil, nil
	}

	table := map[string]function.Function{}
	for providerName := range referenced {
		fns, err := e.resolver.ProviderFunctions(ctx, providerPackageName(config.Terraform, providerName))
		if err != nil {
			if lenient {
				logging.V(5).Infof("provider functions for %q unavailable: %v", providerName, err)
				continue
			}
			return nil, fmt.Errorf("resolving provider functions for %q: %w", providerName, err)
		}
		for tfName, fnSchema := range fns {
			f, err := transform.ProviderFunction(fnSchema.Function, fnSchema.Variadic, e.dryRun,
				e.providerFunctionImpl(ctx, providerName, fnSchema.Function, modInfo))
			if err != nil {
				return nil, fmt.Errorf("projecting function provider::%s::%s: %w", providerName, tfName, err)
			}
			table[ast.ProviderFunctionName(providerName, tfName)] = f
		}
	}
	return table, nil
}

// providerFunctionImpl returns the invoke callback behind a provider-defined
// function. Provider routing mirrors a data source with no explicit
// `provider` argument: the instantiating module call's pass-through
// providers, then the module's own un-aliased provider block, then an
// ancestor's, and otherwise the package's default provider. During preview a
// call whose converted arguments carry unknowns is skipped, which the
// projection turns into an unknown result.
func (e *Engine) providerFunctionImpl(
	ctx context.Context, providerName string, fnSchema *schema.Function, modInfo *graph.ModuleInfo,
) transform.ProviderFunctionImpl {
	return func(args property.Map) (property.Map, error) {
		if e.dryRun && property.New(args).HasComputed() {
			return property.Map{}, nil
		}
		req := InvokeRequest{
			Token:      fnSchema.Token,
			Args:       args,
			PackageRef: e.packageRefs[e.providerPackageName(providerName)],
		}
		if modInfo != nil {
			if ref := e.resolvePassThroughProvider(modInfo, providerName); ref != "" {
				req.Provider = ref
			} else if outputs, ok := e.resourceOutputs.Get(modInfo.Prefix() + providerName); ok {
				if ref, err := providerRefFromCty(outputs); err == nil {
					req.Provider = ref
				}
			} else if ref := e.inheritedDefaultProvider(modInfo, providerName); ref != "" {
				req.Provider = ref
			}
		} else if ref, ok := e.defaultProviders.Get(e.providerPackageName(providerName)); ok {
			req.Provider = ref
		}

		resp, err := e.resmon.Invoke(ctx, req)
		if err != nil {
			return property.Map{}, err
		}
		if len(resp.Failures) > 0 {
			return property.Map{}, fmt.Errorf("%s", strings.Join(resp.Failures, "; "))
		}
		return resp.Return, nil
	}
}

// invokeFunction invokes a Pulumi function (data source).
func (e *Engine) invokeFunction(ctx context.Context, tfType string, req InvokeRequest) (property.Map, error) {
	req, defaults, err := lowerRemoteStateInvoke(tfType, req)
	if err != nil {
		return property.Map{}, err
	}

	resp, err := e.resmon.Invoke(ctx, req)
	if err != nil {
		return property.Map{}, err
	}

	if len(resp.Failures) > 0 {
		return property.Map{}, fmt.Errorf("function invocation failed: %v", resp.Failures)
	}

	return applyRemoteStateDefaults(defaults, resp.Return), nil
}

func (e *Engine) getResourceState(ctx context.Context, ref property.ResourceReference) (property.Map, error) {
	result, err := e.resmon.Invoke(ctx, InvokeRequest{
		Token: "pulumi:pulumi:getResource",
		Args:  property.NewMap(map[string]property.Value{"urn": property.New(string(ref.URN))}),
	})
	if err != nil {
		return property.Map{}, err
	}
	stateVal, ok := result.Return.GetOk("state")
	if !ok || !stateVal.IsMap() {
		return property.Map{}, nil
	}
	return stateVal.AsMap(), nil
}

func (e *Engine) resolveResourceRefsInOutputs(
	ctx context.Context,
	outputs property.Map,
	resSchema *schema.Resource,
) (property.Map, error) {
	resolved := outputs
	for _, p := range resSchema.Properties {
		resType, ok := codegen.UnwrapType(p.Type).(*schema.ResourceType)
		if !ok {
			continue
		}
		v, ok := resolved.GetOk(p.Name)
		if !ok || !v.IsResourceReference() {
			continue
		}
		ref := v.AsResourceReference()
		refMap := property.NewMap(map[string]property.Value{"__ref": property.New(ref)})
		if e.resmon != nil && !ref.ID.IsComputed() && resType.Resource != nil {
			if state, err := e.getResourceState(ctx, ref); err == nil {
				for _, sp := range resType.Resource.Properties {
					if sv, ok := state.GetOk(sp.Name); ok {
						refMap = refMap.Set(sp.Name, sv)
					}
				}
			}
		}
		resolved = resolved.Set(p.Name, property.New(refMap))
	}
	return resolved, nil
}

// resolveConfigResourceReference enriches a resource reference supplied as a
// typed config value with the referenced resource's state, so a program
// consuming this module as a component can read the resource's fields, not just
// its id. The referenced resource lives in the calling program, so its state is
// fetched through the monitor. Best-effort: an unfetchable or preview-unknown
// reference is returned unchanged.
func (e *Engine) resolveConfigResourceReference(ctx context.Context, val cty.Value, u urn.URN) (cty.Value, error) {
	contract.Requiref(e.resmon != nil, "e.resmon", "cannot resolve a resource reference without a resource monitor")
	unmarked, marks := val.Unmark()
	contract.Assertf(unmarked.Type().IsObjectType() && unmarked.Type().HasAttribute("id"),
		"a resource reference must be an object with an id attribute, got %s", unmarked.Type().FriendlyName())

	idAttr, _ := unmarked.GetAttr("id").Unmark()
	if !idAttr.IsKnown() {
		// During preview the referenced resource's id is unknown and its state
		// cannot be fetched; the bare reference flows through unchanged.
		return val, nil
	}
	ref := property.ResourceReference{URN: u, ID: property.New(property.Null)}
	switch {
	case idAttr.IsNull():
		// A component reference has a null id; getResource keys on the URN.
	case idAttr.Type() == cty.String:
		ref.ID = property.New(idAttr.AsString())
	default:
		return cty.Value{}, fmt.Errorf("resource reference %s has a non-string id of type %s",
			u, idAttr.Type().FriendlyName())
	}

	state, err := e.getResourceState(ctx, ref)
	if err != nil {
		return cty.Value{}, fmt.Errorf("fetching state of resource reference %s: %w", u, err)
	}
	attrs := transform.PropertyMapToCty(state).AsValueMap()
	if attrs == nil {
		attrs = map[string]cty.Value{}
	}
	attrs["id"] = unmarked.GetAttr("id")

	obj := cty.ObjectVal(attrs).WithMarks(marks)
	return eval.MarkResourceReference(obj, u), nil
}

// processModule processes a module call.
// Terraform modules map to Pulumi component resources. The module's resources
// become children of the component, and module outputs are collected for references.
// moduleLoaderAdapter adapts modules.Loader to graph.ModuleLoader.
type moduleLoaderAdapter struct {
	loader *modules.Loader
}

func (a *moduleLoaderAdapter) LoadModule(source, version, workDir string) (*graph.LoadedModule, error) {
	// graph.ModuleLoader carries no context, so there is none to thread here.
	loaded, err := a.loader.LoadModule(context.TODO(), source, version, workDir)
	if err != nil {
		return nil, err
	}
	return &graph.LoadedModule{
		Config:     loaded.Config,
		SourcePath: loaded.SourcePath,
	}, nil
}

// forEachModuleInstance iterates over all instances of the module identified by node.ModuleInfo.Prefix().
func (e *Engine) forEachModuleInstance(node *graph.Node, fn func(inst *moduleInstance) error) error {
	instances, ok := e.moduleInstances.Get(node.ModuleInfo.Path)
	if !ok {
		return fmt.Errorf("no module instances for prefix %q", node.ModuleInfo.Prefix())
	}
	for _, inst := range instances {
		if err := fn(inst); err != nil {
			return err
		}
	}
	return nil
}

// processModuleVariable evaluates a module variable's input expression in the parent context
// and stores the result in each module instance's eval context.
func (e *Engine) processModuleVariable(node *graph.Node) error {
	v := node.Variable
	modInfo := node.ModuleInfo
	varName := v.Name

	moduleInputAttrs, _ := modInfo.Module.Config.JustAttributes()
	inputAttr, hasInput := moduleInputAttrs[varName]

	return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		var val cty.Value

		if hasInput {
			// The input expression lives in the enclosing module instance's
			// scope so that expressions like var.name resolve correctly; for
			// root-level calls that is the root evaluator context.
			parentEvalCtx := e.evaluator.Context()
			if inst.Parent != nil {
				parentEvalCtx = inst.Parent.EvalCtx
			}
			var diags hcl.Diagnostics
			hclCtx := parentEvalCtx.HCLContextWithIteration(inst.Index, inst.EachKey, inst.EachVal)
			val, diags = inputAttr.Expr.Value(hclCtx)
			if diags.HasErrors() {
				return fmt.Errorf("evaluating module input %s: %s", varName, diags.Error())
			}
		} else {
			// No input: fall through to default/env/config
			envVarName := "TF_VAR_" + varName
			if envVal := os.Getenv(envVarName); envVal != "" {
				val = cty.StringVal(envVal)
			} else if v.Default != nil {
				var diags hcl.Diagnostics
				val, diags = v.Default.Value(inst.EvalCtx.HCLContext())
				if diags.HasErrors() {
					return fmt.Errorf("evaluating variable default for %s: %s", varName, diags.Error())
				}
			} else {
				return fmt.Errorf("variable %q is required but no value was provided", varName)
			}
		}

		// A `nullable = false` variable rejects an explicit null argument: the
		// default is substituted when one is declared, otherwise it is an
		// error. Matches Terraform/OpenTofu.
		if val.IsNull() && !v.Nullable {
			if v.Default == nil {
				return fmt.Errorf("variable %q must not be set to null: it is declared with nullable = false and has no default", varName)
			}
			var diags hcl.Diagnostics
			val, diags = v.Default.Value(inst.EvalCtx.HCLContext())
			if diags.HasErrors() {
				return fmt.Errorf("evaluating variable default for %s: %s", varName, diags.Error())
			}
		}

		// Fill in optional()-attribute defaults before type conversion so
		// the result satisfies the declared object shape.
		if v.TypeDefaults != nil && !val.IsNull() {
			val = v.TypeDefaults.Apply(val)
		}

		// Coerce the value to match the variable's type constraint.
		if v.TypeConstraint != cty.NilType {
			if converted, err := ctyconvert.Convert(val, v.TypeConstraint); err == nil {
				val = converted
			}
		}

		if v.Sensitive {
			val = val.Mark(eval.SensitiveMark)
		}
		if v.Ephemeral {
			val = val.Mark(eval.EphemeralMark)
		}

		inst.EvalCtx.SetVariable(varName, val)

		return runVariableValidations(eval.NewEvaluator(inst.EvalCtx), varName, v.Validations)
	})
}

// processModuleInit processes a module init node: registers component resources and creates instances.
func (e *Engine) processModuleInit(ctx context.Context, node *graph.Node) error {
	modInfo := node.ModuleInfo
	mod := modInfo.Module

	componentType := fmt.Sprintf("components:index:%s", componentTypeName(modInfo.SourcePath))

	// A nested module call runs once per instance of the enclosing module; a
	// root-level call runs in the single root scope (a nil parent instance).
	// When the parent has zero instances (count=0 / for_each empty) the
	// entire inner subtree must be skipped — registering an empty instances
	// slice lets downstream per-instance work (vars, locals, nested modules,
	// resources) loop zero times instead of falling back to the root context.
	parents := []*moduleInstance{nil}
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPath())
		if !ok || len(parentInstances) == 0 {
			e.moduleInstances.Set(modInfo.Path, nil)
			return nil
		}
		parents = parentInstances
	}

	// Load the child module to get variable type constraints for input coercion.
	loaderWorkDir := modInfo.ParentSourcePath
	if loaderWorkDir == "" {
		loaderWorkDir = e.workDir
	}
	childMod, err := e.moduleLoader.LoadModule(ctx, mod.Source, mod.Version, loaderWorkDir)
	if err != nil {
		return fmt.Errorf("loading module %s for input types: %w", mod.Source, err)
	}

	// One table serves every instance: the functions a module can call depend
	// on its config and call site, not on the count/for_each iteration.
	moduleFunctions, err := e.providerFunctionTable(ctx, childMod.Config, modInfo)
	if err != nil {
		return fmt.Errorf("module %s: %w", mod.Source, err)
	}

	var instances []*moduleInstance
	for _, parent := range parents {
		insts, err := e.initModuleCallIn(ctx, node, childMod, moduleFunctions, componentType, parent)
		if err != nil {
			return err
		}
		instances = append(instances, insts...)
	}
	e.moduleInstances.Set(modInfo.Path, instances)
	return nil
}

// initModuleCallIn creates the instances of one module call within a single
// instance of the enclosing module (nil parent = the root config): it
// evaluates the call's inputs and count/for_each in that instance's scope and
// registers one component resource per resulting instance.
func (e *Engine) initModuleCallIn(
	ctx context.Context, node *graph.Node, childMod *modules.LoadedModule,
	moduleFunctions map[string]function.Function, componentType string,
	parent *moduleInstance,
) ([]*moduleInstance, error) {
	modInfo := node.ModuleInfo
	mod := modInfo.Module

	parentURN, parentEvalCtx, parentPath, parentName := e.stackURN, e.evaluator.Context(), modulepath.Root(), ""
	if parent != nil {
		parentURN, parentEvalCtx, parentPath, parentName = parent.URN, parent.EvalCtx, parent.Path, parent.Name
	}

	// Evaluate module inputs for the component resource registration
	inputs := make(map[string]property.Value)
	attrs, _ := mod.Config.JustAttributes()
	for name, attr := range attrs {
		val, diags := attr.Expr.Value(parentEvalCtx.HCLContext())
		if diags.HasErrors() {
			continue
		}
		// Coerce to the variable's declared type if available.
		if v, ok := childMod.Config.Variables[name]; ok && v.TypeConstraint != cty.NilType {
			if converted, convErr := ctyconvert.Convert(val, v.TypeConstraint); convErr == nil {
				val = converted
			}
		}
		pv, err := transform.CtyToPropertyValue(val)
		if err == nil {
			inputs[name] = pv
		}
	}

	newInstance := func(index *int, eachKey, eachVal *cty.Value) (*moduleInstance, error) {
		instPath := instancePath(parentPath, modInfo.ModuleName(), index, eachKey)
		instName, err := moduleInstanceName(mod, parentEvalCtx, parentName, instPath, index, eachKey, eachVal)
		if err != nil {
			return nil, err
		}
		componentOpts := &ResourceOptions{Parent: parentURN}
		componentOpts.Aliases = e.moduleComponentAliases(instPath)
		componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instName, property.NewMap(inputs), componentOpts)
		if err != nil {
			return nil, fmt.Errorf("registering module component %s: %w", instPath.String(), err)
		}
		instCtx, err := newEvalContext(
			e.absolutePaths, modInfo.SourcePath, e.workDir, e.workDir,
			e.stackName, e.projectName, e.organization,
		)
		if err != nil {
			return nil, fmt.Errorf("creating the module evaluation context: %w", err)
		}
		instCtx.SetProviderFunctions(moduleFunctions)
		instCtx.SetModuleName(instName)
		return &moduleInstance{
			Path:       instPath,
			ModuleInfo: modInfo,
			Name:       instName,
			EvalCtx:    instCtx,
			URN:        componentURN,
			Parent:     parent,
			Index:      index,
			EachKey:    eachKey,
			EachVal:    eachVal,
			Outputs:    make(map[string]cty.Value),
		}, nil
	}

	// No count/for_each: single instance.
	if mod.Count == nil && mod.ForEach == nil {
		inst, err := newInstance(nil, nil, nil)
		if err != nil {
			return nil, err
		}
		return []*moduleInstance{inst}, nil
	}

	if mod.Count != nil {
		count, _, unknown, _, diags := eval.NewEvaluator(parentEvalCtx).EvaluateCount(mod.Count)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating module count: %s", diags.Error())
		}
		if unknown {
			// TODO: support unknown module expansion during preview the way
			// resources do (register no instances, bind outputs to unknown).
			return nil, fmt.Errorf("%s: the count value depends on values that are not yet known", node.Key)
		}
		var instances []*moduleInstance
		for idx := range count {
			inst, err := newInstance(&idx, nil, nil)
			if err != nil {
				return nil, err
			}
			inst.EvalCtx.SetCount(idx)
			instances = append(instances, inst)
		}
		return instances, nil
	}

	forEach, unknown, _, diags := eval.NewEvaluator(parentEvalCtx).EvaluateForEach(mod.ForEach)
	if diags.HasErrors() {
		return nil, fmt.Errorf("evaluating module for_each: %s", diags.Error())
	}
	if unknown {
		// TODO: support unknown module expansion during preview the way
		// resources do (register no instances, bind outputs to unknown).
		return nil, fmt.Errorf("%s: the for_each value depends on values that are not yet known", node.Key)
	}

	var instances []*moduleInstance
	for _, ks := range slices.Sorted(maps.Keys(forEach)) {
		k := cty.StringVal(ks)
		v := forEach[ks]
		inst, err := newInstance(nil, &k, &v)
		if err != nil {
			return nil, err
		}
		inst.EvalCtx.SetEach(k, v)
		instances = append(instances, inst)
	}
	return instances, nil
}

// processModuleOutput evaluates a module output in each instance and stores it in the parent context.
func (e *Engine) processModuleOutput(_ context.Context, node *graph.Node) error {
	output := node.Output
	modInfo := node.ModuleInfo
	outputName := strings.TrimPrefix(node.Key, modInfo.Prefix()+"output.")

	err := e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		if err := runOutputPreconditions(output, inst.EvalCtx.HCLContext(), outputName); err != nil {
			return err
		}
		val, diags := output.Value.Value(inst.EvalCtx.HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module output %s: %s", outputName, diags.Error())
		}
		// A `sensitive = true` output carries the mark into the calling module,
		// so a reference to it stays sensitive; likewise for `ephemeral = true`.
		if output.Sensitive {
			val = val.Mark(eval.SensitiveMark)
		}
		if output.Ephemeral {
			val = val.Mark(eval.EphemeralMark)
		}
		inst.mu.Lock()
		inst.Outputs[outputName] = val
		inst.mu.Unlock()
		return nil
	})
	if err != nil {
		return err
	}

	// Eagerly publish outputs to the parent contexts so other module variables
	// can reference them before the completion node runs.
	instances, ok := e.moduleInstances.Get(modInfo.Path)
	if !ok {
		return nil
	}
	e.publishModuleValue(modInfo, instances)
	return nil
}

// publishModuleValue assembles the value of `module.<name>` from the
// instances' collected outputs and publishes it into each enclosing module
// instance's eval context (or the root context for a top-level call). Each
// enclosing instance sees only its own instances of the call.
func (e *Engine) publishModuleValue(modInfo *graph.ModuleInfo, instances []*moduleInstance) {
	parents := []*moduleInstance{nil}
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPath())
		if !ok {
			return
		}
		parents = parentInstances
	}

	mod := modInfo.Module
	name := modInfo.ModuleName()
	for _, parent := range parents {
		parentCtx := e.evaluator.Context()
		if parent != nil {
			parentCtx = parent.EvalCtx
		}
		var children []*moduleInstance
		for _, inst := range instances {
			if inst.Parent == parent {
				children = append(children, inst)
			}
		}

		switch {
		case mod.Count != nil:
			tupleVals := make([]cty.Value, len(children))
			for i, inst := range children {
				tupleVals[i] = inst.outputObject()
			}
			if len(tupleVals) > 0 {
				parentCtx.SetModule(name, cty.TupleVal(tupleVals))
			} else {
				parentCtx.SetModule(name, cty.EmptyTupleVal)
			}
		case mod.ForEach != nil:
			mapVals := make(map[string]cty.Value, len(children))
			for _, inst := range children {
				if inst.EachKey == nil {
					continue
				}
				mapVals[inst.EachKey.AsString()] = inst.outputObject()
			}
			if len(mapVals) > 0 {
				parentCtx.SetModule(name, cty.ObjectVal(mapVals))
			} else {
				parentCtx.SetModule(name, cty.EmptyObjectVal)
			}
		default:
			if len(children) == 1 {
				inst := children[0]
				inst.mu.Lock()
				outs := maps.Clone(inst.Outputs)
				inst.mu.Unlock()
				if len(outs) == 0 {
					parentCtx.SetModule(name, cty.EmptyObjectVal)
				}
				for k, v := range outs {
					parentCtx.SetModuleOutput(name, k, v)
				}
			}
		}
	}
}

// processModuleComplete handles the module completion node: registers component outputs
// and assembles the full module value in the parent context.
func (e *Engine) processModuleComplete(ctx context.Context, node *graph.Node) error {
	modInfo := node.ModuleInfo
	if modInfo == nil {
		return fmt.Errorf("module completion node missing ModuleInfo")
	}

	instances, ok := e.moduleInstances.Get(modInfo.Path)
	if !ok {
		return fmt.Errorf("no module instances for prefix %q", modInfo.Prefix())
	}

	// Register component outputs and collect per-instance output objects.
	for _, inst := range instances {
		if e.resmon != nil {
			outputProps := make(map[string]property.Value)
			for k, v := range inst.Outputs {
				pv, err := transform.CtyToPropertyValue(v)
				if err == nil {
					outputProps[k] = pv
				}
			}
			if err := e.resmon.RegisterResourceOutputs(ctx, inst.URN, property.NewMap(outputProps)); err != nil {
				return fmt.Errorf("registering module outputs: %w", err)
			}
		}
	}

	e.publishModuleValue(modInfo, instances)
	return nil
}

// componentTypeName derives a component type name from its source directory path,
// replicating PCL's DeclarationName logic.
func componentTypeName(sourcePath string) string {
	name := filepath.Base(sourcePath)
	for _, ch := range []string{"-", ".", " "} {
		name = strings.ReplaceAll(name, ch, "_")
	}
	parts := strings.Split(name, "_")
	var b strings.Builder
	for _, p := range parts {
		if p != "" {
			b.WriteString(strings.ToUpper(p[:1]) + p[1:])
		}
	}
	return b.String()
}

// registerComponentResource registers a component (non-custom) resource.
func (e *Engine) registerComponentResource(
	ctx context.Context,
	typeToken string,
	name string,
	inputs property.Map,
	opts *ResourceOptions,
) (urn.URN, string, property.Map, error) {
	if e.resmon == nil {
		urn := urn.New(tokens.QName(e.stackName), tokens.PackageName(e.projectName),
			"", tokens.Type(typeToken), name)
		return urn, "", inputs, nil
	}

	deps := opts.DependsOn
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:         typeToken,
		Name:         name,
		Inputs:       inputs,
		Dependencies: deps,
		Parent:       opts.Parent,
	})
	if err != nil {
		return "", "", property.Map{}, err
	}

	return resp.URN, resp.ID, resp.Outputs, nil
}

// processOutput processes an output definition.
func (e *Engine) processOutput(_ context.Context, name string, output *ast.Output) error {
	if err := runOutputPreconditions(output, e.evaluator.Context().HCLContext(), name); err != nil {
		return err
	}

	// Evaluate the output value, intercepting can() calls.
	val, diags := e.evaluator.EvaluateExpression(output.Value)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating output value: %s", diags.Error())
	}

	// Root outputs that evaluate to null are removed entirely; nothing can
	// reference a root output, so omitting it is always safe.
	if val.IsNull() {
		return nil
	}

	// Convert to PropertyValue
	pv, err := transform.CtyToPropertyValue(val)
	if err != nil {
		return fmt.Errorf("converting output value: %w", err)
	}

	// Sensitive and ephemeral outputs are both persisted as secrets.
	if output.Sensitive || output.Ephemeral {
		pv = pv.WithSecret(true)
	}

	// Store the output for later registration on the stack
	e.stackOutputs[name] = pv

	return nil
}

// providerRefFromCty extracts a "<urn>::<id>" provider reference from an evaluated
// `provider` attribute value.
//
// The value must be one of:
//   - a resource-outputs object with direct `urn` and `id` string attributes
//     (provider blocks like `aws.west` or pulumi_providers_* resources), or
//   - a resource reference carrying its URN in a resourceMark (e.g., the result
//     of a `call.<resource>.<method>` that returns a provider), with its id in
//     the object's id attribute.
func providerRefFromCty(val cty.Value) (string, error) {
	// Capture the reference's URN before the unmark below strips its resourceMark.
	refURN, isRef := eval.ResourceReferenceURN(val)

	if val.IsMarked() {
		val, _ = val.Unmark()
	}
	if !val.IsKnown() {
		return "", errors.New("provider value is not yet known")
	}
	if val.IsNull() {
		return "", errors.New("provider value is null")
	}
	if !val.Type().IsObjectType() {
		return "", fmt.Errorf("provider value must be an object, got %s", val.Type().FriendlyName())
	}
	if val.Type().HasAttribute("urn") && val.Type().HasAttribute("id") {
		urn := ctyAsString(val.GetAttr("urn"))
		id := ctyAsString(val.GetAttr("id"))
		if urn == "" || id == "" {
			return "", fmt.Errorf("provider value urn/id must be non-empty strings, got urn=%q id=%q", urn, id)
		}
		return urn + "::" + id, nil
	}
	if isRef {
		id := ""
		if val.Type().HasAttribute("id") {
			if idAttr, _ := val.GetAttr("id").Unmark(); idAttr.Type() == cty.String && idAttr.IsKnown() && !idAttr.IsNull() {
				id = idAttr.AsString()
			}
		}
		return string(refURN) + "::" + id, nil
	}
	return "", errors.New("provider value is not a resource reference")
}

// Validate validates an HCL configuration without executing it.
func Validate(config *ast.Config) []error {
	var errs []error

	g, err := graph.BuildFromConfig(config, nil, "")
	if err != nil {
		errs = append(errs, err)
		return errs
	}

	errs = append(errs, g.Validate()...)

	// Additional validation
	// TODO: Type checking, schema validation, etc.

	return errs
}

// checkRuleDeps collects the URNs of the resources referenced by the condition
// and error_message expressions of the given check rules. A reference in a
// precondition/postcondition establishes a dependency, as in TF, even though
// the rules themselves are only evaluated later by hooks. Each referenced
// variable is resolved through the eval context instead of evaluating the
// expression, so no function fires early while values reached through locals
// still carry their DepMarks. Traversals that do not resolve are skipped —
// notably self, which is only in scope once a postcondition hook fires.
func checkRuleDeps(rules []*ast.CheckRule, hclCtx *hcl.EvalContext) []string {
	var deps []string
	for _, rule := range rules {
		for _, expr := range []hcl.Expression{rule.Condition, rule.ErrorMessage} {
			if expr == nil {
				continue
			}
			for _, traversal := range expr.Variables() {
				val, diags := traversal.TraverseAbs(hclCtx)
				if diags.HasErrors() {
					continue
				}
				deps = append(deps, eval.CollectDepURNs(val)...)
			}
		}
	}
	return deps
}

func provisionerDeps(res *ast.Resource, hclCtx *hcl.EvalContext) []string {
	var deps []string
	var collect func(body hcl.Body)
	collect = func(body hcl.Body) {
		if body == nil {
			return
		}
		if eb, ok := body.(*ast.EscapedBody); ok {
			collect(eb.Base)
			collect(eb.Escape)
			return
		}
		attrs, _ := body.JustAttributes()
		for _, attr := range attrs {
			for _, traversal := range attr.Expr.Variables() {
				val, diags := traversal.TraverseAbs(hclCtx)
				if diags.HasErrors() {
					continue
				}
				deps = append(deps, eval.CollectDepURNs(val)...)
			}
		}
		if syntaxBody, ok := body.(*hclsyntax.Body); ok {
			for _, block := range syntaxBody.Blocks {
				collect(block.Body)
			}
		}
	}
	if res.Connection != nil {
		collect(res.Connection.Config)
	}
	for _, prov := range res.Provisioners {
		if prov.When == "destroy" {
			continue
		}
		if prov.Connection != nil {
			collect(prov.Connection.Config)
		}
		collect(prov.Config)
	}
	return deps
}

// bindPreconditionHooks registers a hook per precondition and binds it to
// BeforeCreate/BeforeUpdate so a false condition blocks the operation.
//
// The HCL eval context is snapshotted at registration so the async callback
// sees the per-instance count/each/self that was in scope here — by the time
// the callback fires, processing has moved on. Other resources' outputs are
// pinned too; graph order guarantees all referenced resources are settled
// by registration time. Unknown values defer (return nil) per TF's
// "known after apply" semantics.
func (e *Engine) bindPreconditionHooks(
	ctx context.Context,
	res *ast.Resource,
	instance *graph.ExpandedResource,
	evalCtx *eval.Context,
	opts *ResourceOptions,
	resourceName string,
) error {
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	for i, rule := range res.Preconditions {
		rule, index := rule, i+1
		hookName := fmt.Sprintf("%s.%s:precondition:%d", res.Type, resourceName, i)
		callback := func(_ context.Context, _ *ResourceHookArgs) error {
			return evaluatePrecondition(rule, hclSnapshot, index, instance.Key)
		}
		if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
			OnDryRun: true,
		}); err != nil {
			return fmt.Errorf("registering precondition hook: %w", err)
		}
		opts.Hooks.BeforeCreate = append(opts.Hooks.BeforeCreate, hookName)
		opts.Hooks.BeforeUpdate = append(opts.Hooks.BeforeUpdate, hookName)
	}
	return nil
}

// bindPostconditionHooks registers a hook per postcondition and binds it to
// AfterCreate/AfterUpdate. The hook fires after the resource is created or
// updated; the engine supplies the resource's new outputs as `self` in the
// callback. Failed postconditions surface as deployment errors but do not
// unwind the resource registration — same as TF.
func (e *Engine) bindPostconditionHooks(
	ctx context.Context,
	res *ast.Resource,
	resSchema *schema.Resource,
	mapping *bridge.BodyMapping,
	instance *graph.ExpandedResource,
	evalCtx *eval.Context,
	opts *ResourceOptions,
	resourceName string,
) error {
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	dryRun := e.dryRun
	for i, rule := range res.Postconditions {
		rule, index := rule, i+1
		hookName := fmt.Sprintf("%s.%s:postcondition:%d", res.Type, resourceName, i)
		callback := func(_ context.Context, args *ResourceHookArgs) error {
			// Hooks receive raw engine outputs, so terraform_data's surface is
			// adapted the same way as on the registration path: property-level
			// lowering here, wrapper unboxing after re-expansion.
			outputs := lowerTerraformDataOutputs(res.Type, args.NewOutputs, opts)
			return evaluatePostcondition(rule, hclSnapshot, outputs, res.Type, resSchema, mapping, dryRun, index, instance.Key)
		}
		if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
			OnDryRun: true,
		}); err != nil {
			return fmt.Errorf("registering postcondition hook: %w", err)
		}
		opts.Hooks.AfterCreate = append(opts.Hooks.AfterCreate, hookName)
		opts.Hooks.AfterUpdate = append(opts.Hooks.AfterUpdate, hookName)
	}
	return nil
}

// evaluatePostcondition evaluates a postcondition with `self` bound to the
// engine-supplied NewOutputs.
func evaluatePostcondition(
	rule *ast.CheckRule, hclCtx *hcl.EvalContext, newOutputs property.Map, tfType string,
	resSchema *schema.Resource, mapping *bridge.BodyMapping, dryRun bool, index int, resourceName string,
) error {
	outputObj, err := transform.ResourceOutputToCty(newOutputs, resSchema, mapping, dryRun)
	if err != nil {
		return fmt.Errorf("converting outputs for postcondition %d on %s: %w", index, resourceName, err)
	}
	if err := unwrapTerraformDataOutputs(tfType, outputObj, newOutputs); err != nil {
		return fmt.Errorf("converting outputs for postcondition %d on %s: %w", index, resourceName, err)
	}
	return evaluatePostconditionValue(rule, hclCtx, cty.ObjectVal(outputObj), index, resourceName)
}

// evaluatePostconditionValue evaluates a postcondition with `self` bound to
// the given value. Unknown conditions defer (return nil).
func evaluatePostconditionValue(
	rule *ast.CheckRule, hclCtx *hcl.EvalContext, self cty.Value, index int, resourceName string,
) error {
	selfCtx := hclCtx.NewChild()
	selfCtx.Variables = map[string]cty.Value{"self": self}
	condVal, diags := rule.Condition.Value(selfCtx)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating postcondition %d for %s: %s", index, resourceName, diags.Error())
	}
	condVal, _ = condVal.Unmark()
	if !condVal.IsKnown() {
		return nil
	}
	ok, err := conditionResultToBool(condVal)
	if err != nil {
		return fmt.Errorf("postcondition %d for %s: %s", index, resourceName, err)
	}
	if ok {
		return nil
	}
	msgVal, msgDiags := rule.ErrorMessage.Value(selfCtx)
	if msgDiags.HasErrors() {
		return fmt.Errorf("postcondition %d for %s failed (could not evaluate error message: %s)",
			index, resourceName, msgDiags.Error())
	}
	msg := "postcondition check failed"
	if s := renderErrorMessage(msgVal); s != "" {
		msg = s
	}
	return fmt.Errorf("postcondition for %s: %s", resourceName, msg)
}

// conditionResultToBool mirrors OpenTofu's handling of a condition result
// (variable validation, precondition, or postcondition). OpenTofu converts the
// result to bool, so any value convertible to bool — notably the strings
// "true"/"false" — is a valid condition value, not a type error. A null result
// is rejected, matching OpenTofu's "must return either true or false, not null".
func conditionResultToBool(v cty.Value) (bool, error) {
	converted, err := ctyconvert.Convert(v, cty.Bool)
	if err != nil {
		return false, fmt.Errorf("condition must be a boolean: %s", err)
	}
	if converted.IsNull() {
		return false, fmt.Errorf("condition must return either true or false, not null")
	}
	return converted.True(), nil
}

// runOutputPreconditions evaluates an output's `precondition` rules against
// hclCtx and returns an error for the first rule whose condition is known and
// false; unknown conditions are deferred.
func runOutputPreconditions(output *ast.Output, hclCtx *hcl.EvalContext, name string) error {
	for i, rule := range output.Preconditions {
		if err := evaluatePrecondition(rule, hclCtx, i+1, fmt.Sprintf("output %q", name)); err != nil {
			return err
		}
	}
	return nil
}

// evaluatePrecondition returns nil when the rule holds or its condition is
// unknown (deferred), or a formatted error when it fails.
func evaluatePrecondition(rule *ast.CheckRule, hclCtx *hcl.EvalContext, index int, resourceName string) error {
	condVal, diags := rule.Condition.Value(hclCtx)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating precondition %d for %s: %s", index, resourceName, diags.Error())
	}
	condVal, _ = condVal.Unmark()
	if !condVal.IsKnown() {
		return nil
	}
	ok, err := conditionResultToBool(condVal)
	if err != nil {
		return fmt.Errorf("precondition %d for %s: %s", index, resourceName, err)
	}
	if ok {
		return nil
	}
	msgVal, msgDiags := rule.ErrorMessage.Value(hclCtx)
	if msgDiags.HasErrors() {
		return fmt.Errorf("precondition %d for %s failed (could not evaluate error message: %s)",
			index, resourceName, msgDiags.Error())
	}
	msg := "precondition check failed"
	if s := renderErrorMessage(msgVal); s != "" {
		msg = s
	}
	return fmt.Errorf("precondition for %s: %s", resourceName, msg)
}

// evaluateChecks evaluates every check block after all resources have settled.
// Both scoped-data-source errors and failed assertions emit warnings and
// processing continues, matching Terraform: check blocks are the only custom
// condition that does not block the operation.
func (e *Engine) evaluateChecks(ctx context.Context) error {
	contract.Assertf(e.resmon != nil, "e.resmon cannot be nil")
	names := slices.Collect(maps.Keys(e.config.Checks))
	slices.Sort(names)
	var errs []error
	for _, name := range names {
		errs = append(errs, e.evaluateCheck(ctx, name, e.config.Checks[name]))
	}
	return errors.Join(errs...)
}

// evaluateCheck evaluates one check block. Its scoped data source is read into a
// cloned context so it is visible only to this check's assertions and cannot
// collide with a top-level data source of the same address.
func (e *Engine) evaluateCheck(ctx context.Context, name string, check *ast.Check) error {
	evalCtx := e.evaluator.Context()
	var errs []error
	if ds := check.DataResource; ds != nil {
		evalCtx = evalCtx.Clone()
		if err := e.readScopedDataSource(ctx, ds, evalCtx); err != nil {
			// A scoped data source error is masked as a warning, matching TF.
			errs = append(errs, e.warnf(ctx, "check %q data %q.%q: %s", name, ds.Type, ds.Name, err))
		}
	}
	evaluator := eval.NewEvaluator(evalCtx)
	for _, assert := range check.Asserts {
		if msg := evaluateCheckAssert(evaluator, assert); msg != "" {
			errs = append(errs, e.warnf(ctx, "check %q assertion failed: %s", name, msg))
		}
	}
	return errors.Join(errs...)
}

// readScopedDataSource reads a check's scoped data source into evalCtx, making
// it available as data.<type>.<name> within that context only.
func (e *Engine) readScopedDataSource(ctx context.Context, ds *ast.Resource, evalCtx *eval.Context) error {
	node := &graph.Node{
		Key:      "data." + ast.ResourceKey(ds.Type, ds.Name),
		Type:     graph.NodeTypeDataSource,
		Resource: ds,
	}
	return e.processDataSourceInContext(ctx, node, ds, evalCtx)
}

// warnf emits a best-effort warning diagnostic. A transport failure emitting it
// must not turn a non-blocking check into a failed operation.
func (e *Engine) warnf(ctx context.Context, format string, args ...any) error {
	return e.resmon.LogWarning(ctx, fmt.Sprintf(format, args...))
}

// evaluateCheckAssert evaluates a single check assertion. It returns the
// assertion's error message when the condition is known and false (or its
// condition cannot be evaluated, which Terraform masks as a warning), and ""
// when the assertion holds or its condition is not yet known.
func evaluateCheckAssert(evaluator *eval.Evaluator, rule *ast.CheckRule) string {
	condVal, diags := evaluator.EvaluateExpression(rule.Condition)
	if diags.HasErrors() {
		return fmt.Sprintf("could not evaluate condition: %s", diags.Error())
	}
	condVal, _ = condVal.Unmark()
	if !condVal.IsKnown() {
		return ""
	}
	ok, err := conditionResultToBool(condVal)
	if err != nil {
		return err.Error()
	}
	if ok {
		return ""
	}
	msgVal, msgDiags := evaluator.EvaluateExpression(rule.ErrorMessage)
	if msgDiags.HasErrors() {
		return "assertion failed (could not evaluate error message)"
	}
	if msg := renderErrorMessage(msgVal); msg != "" {
		return msg
	}
	return "assertion failed"
}

// checkPulumiVersion checks if the Pulumi CLI version satisfies the required version range.
// The version requirement is specified via the pulumi block's requiredVersionRange attribute.
func (e *Engine) checkPulumiVersion(ctx context.Context) error {
	// Check if the pulumi block exists and has a version requirement
	if e.config.Terraform == nil || e.config.Terraform.RequiredVersionRange == nil {
		// No version requirement specified
		return nil
	}

	// Evaluate the requiredVersionRange expression
	versionVal, diags := e.evaluator.EvaluateExpression(e.config.Terraform.RequiredVersionRange)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating requiredVersionRange: %s", diags.Error())
	}

	// Get the version range string
	if versionVal.Type() != cty.String {
		return fmt.Errorf("requiredVersionRange must be a string, got %s", versionVal.Type().FriendlyName())
	}

	versionRange := versionVal.AsString()
	if versionRange == "" {
		return nil
	}

	return e.resmon.CheckPulumiVersion(ctx, versionRange)
}

func ptr[T any](v T) *T { return &v }

// ctyAsString reads a cty value as a string, tolerating marks (resource
// output leaves carry DepMarks) and returning "" for null / unknown /
// non-string. Use the cty API directly if you need to distinguish those.
// resourceURNsFromValue extracts the URNs of the resource instances val
// represents when it is a whole-resource value: a single instance carries a
// resource reference mark, while a reference to a resource with count or
// for_each yields a tuple or object of such instance values. An attribute of a
// resource inherits the mark but not its hash, so it yields no URNs.
func resourceURNsFromValue(val cty.Value) []string {
	if u, ok := eval.ResourceReferenceURN(val); ok {
		return []string{string(u)}
	}
	val, _ = val.Unmark()
	if val.IsNull() || !val.IsKnown() || !val.CanIterateElements() {
		return nil
	}
	var urns []string
	for it := val.ElementIterator(); it.Next(); {
		_, el := it.Element()
		if u, ok := eval.ResourceReferenceURN(el); ok {
			urns = append(urns, string(u))
		}
	}
	return urns
}

func ctyAsString(v cty.Value) string {
	if v.IsMarked() {
		v, _ = v.Unmark()
	}
	if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}

// sensitiveErrorMessageRef is the text substituted for a custom-condition
// error_message that interpolates a sensitive value. Rendering the real message
// would leak the secret, so the reference is reported instead.
const sensitiveErrorMessageRef = "Error message refers to sensitive values"

// renderErrorMessage reads a custom-condition error_message (variable
// validation, precondition, postcondition, or check assertion). It refuses to
// render a message that carries the sensitive mark — returning
// sensitiveErrorMessageRef rather than the interpolated secret — while still
// tolerating the non-sensitive marks (e.g. DepMarks) that resource outputs
// carry. A null / unknown / non-string value yields "" so the caller can fall
// back to its own default message.
func renderErrorMessage(v cty.Value) string {
	if v.HasMark(eval.SensitiveMark) || v.HasMark(eval.EphemeralMark) {
		return sensitiveErrorMessageRef
	}
	return ctyAsString(v)
}
