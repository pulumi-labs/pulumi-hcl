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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/graph"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modules"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi-labs/pulumi-hcl/pkg/util"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	ctyconvert "github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"github.com/zclconf/go-cty/cty/json"
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

	// Invoke invokes a provider function.
	Invoke(ctx context.Context, req InvokeRequest) (*InvokeResponse, error)

	// Call invokes a method on a resource.
	Call(ctx context.Context, req CallRequest) (*CallResponse, error)

	// RegisterResourceOutputs registers outputs on a resource (used for stack outputs).
	RegisterResourceOutputs(ctx context.Context, urn string, outputs property.Map) error

	// CheckPulumiVersion checks if the Pulumi CLI version satisfies the given version range.
	CheckPulumiVersion(ctx context.Context, versionRange string) error

	// RegisterResourceHook registers a named callback. Must be called before any
	// resource registration that binds the hook by name via Hooks.
	RegisterResourceHook(ctx context.Context, name string, callback ResourceHookFunction, opts ResourceHookOptions) error
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
	IgnoreChanges           []string
	Aliases                 []Alias
	Provider                string
	Providers               map[string]string // Map from package name to provider reference (urn::id)
	Parent                  string
	DeleteBeforeReplace     bool
	DeleteBeforeReplaceDef  bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts          *CustomTimeouts
	ImportId                string // Resource ID to import
	AdditionalSecretOutputs []string
	RetainOnDelete          *bool
	DeletedWith             string         // URN of the resource that, when deleted, causes this resource to be deleted
	ReplaceWith             []string       // URNs of resources whose replacement triggers replacement of this resource
	HideDiffs               []string       // Property paths whose diffs should not be displayed
	ReplaceOnChanges        []string       // Property paths that if changed should force a replacement
	ReplacementTrigger      property.Value // Value whose change triggers replacement
	EnvVarMappings          map[string]string
	Version                 string
	PluginDownloadURL       string
	PackageRef              PackageRef
	Hooks                   *ResourceHookBinding
}

// RegisterResourceResponse contains the result of registering a resource.
type RegisterResourceResponse struct {
	URN     string
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
	// any. Path.LogicalName() is the canonical Pulumi component name.
	Path    modulepath.Path
	EvalCtx *eval.Context        // per-instance evaluation context
	URN     string               // component URN
	Index   *int                 // count index (nil if not using count)
	EachKey *cty.Value           // for_each key (nil if not using for_each)
	EachVal *cty.Value           // for_each value (nil if not using for_each)
	mu      sync.Mutex           // protects Outputs
	Outputs map[string]cty.Value // collected output values
}

// instancePath builds the path for one instance of a module call, replacing
// the leaf step with one that carries the runtime disambiguator (count
// index or for_each key, if any).
func instancePath(modInfo *graph.ModuleInfo, index *int, eachKey *cty.Value) modulepath.Path {
	parent, leaf, ok := modInfo.Path.Parent()
	if !ok {
		return modInfo.Path
	}
	switch {
	case index != nil:
		return parent.Append(modulepath.NewIndexedStep(leaf.Name(), *index))
	case eachKey != nil:
		return parent.Append(modulepath.NewKeyedStep(leaf.Name(), eachKey.AsString()))
	default:
		return modInfo.Path
	}
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
	stackURN string

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

	// pulumiConfig contains Pulumi stack configuration values.
	pulumiConfig map[string]string

	// configSecretKeys lists keys that should be treated as secrets.
	configSecretKeys []string

	// moduleLoader loads and caches module configurations.
	moduleLoader *modules.Loader

	// moduleInstances maps module prefix → list of instances for inlined modules.
	moduleInstances *util.SyncMap[string, []*moduleInstance]

	parallel int

	// failedNodes tracks resource nodes that failed to register, keyed by instance key.
	// Dependent nodes check this map and are skipped when a dependency failed.
	failedNodes *util.SyncMap[string, error]

	// graph is the resolved dependency graph for the current run. Stored on
	// the engine so processNode handlers can consult its topology (e.g.
	// processProvider checks whether anything depends on a provider node
	// before registering it).
	graph *graph.Graph

	// alwaysRegisterProviders forces every `provider` block to be registered
	// as a resource even when nothing references it, bypassing Terraform's
	// lazy provider-configure semantics. Test-only; see
	// EngineOptions.AlwaysRegisterProviders.
	alwaysRegisterProviders bool
}

// EngineOptions configures the engine.
type EngineOptions struct {
	// ProjectName is the Pulumi project name.
	ProjectName string

	// StackName is the Pulumi stack name.
	StackName string

	// Organization is the Pulumi organization name.
	Organization string

	// Config contains the Pulumi configuration values.
	Config map[string]string

	// ConfigSecretKeys lists keys that should be treated as secrets.
	ConfigSecretKeys []string

	// DryRun indicates this is a preview operation.
	DryRun bool

	// ResourceMonitor is the resource monitor for registering resources.
	ResourceMonitor ResourceMonitor

	// WorkDir is the working directory (where the program files are).
	WorkDir string

	// RootDir is the project root directory (where Pulumi.yaml is).
	RootDir string

	SchemaLoader schema.ReferenceLoader

	// ProviderInfoSource is the bridge mapping resolver. Optional; when nil
	// the engine falls back to convention-based name mapping.
	ProviderInfoSource bridge.ProviderInfoSource

	// Packages maps parameterized package alias to its descriptor.
	// The engine calls RegisterPackage on the resource monitor for each entry before running the program.
	Packages map[string]workspace.PackageDescriptor

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
func NewEngine(ctx context.Context, config *ast.Config, opts *EngineOptions) *Engine {
	contract.Assertf(opts.SchemaLoader != nil, "EngineOptions.SchemaLoader cannot be nil")
	contract.Assertf(opts.WorkDir != "", "EngineOptions.WorkDir cannot be empty")
	contract.Assertf(opts.RootDir != "", "EngineOptions.RootDir cannot be empty")

	evalCtx := eval.NewContext(opts.WorkDir, opts.RootDir, opts.WorkDir,
		opts.StackName, opts.ProjectName, opts.Organization)

	engine := &Engine{
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
		pulumiConfig:            opts.Config,
		configSecretKeys:        opts.ConfigSecretKeys,
		packages:                opts.Packages,
		packageRefs:             make(map[string]PackageRef),
		moduleLoader:            modules.NewLoader(ctx),
		moduleInstances:         util.NewSyncMap[string, []*moduleInstance](),
		parallel:                opts.Parallel,
		failedNodes:             util.NewSyncMap[string, error](),
		alwaysRegisterProviders: opts.AlwaysRegisterProviders,
	}

	return engine
}

// Run executes the HCL program.
func (e *Engine) Run(ctx context.Context) error {
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

	// Process nodes in parallel where possible
	if err := e.processGraph(ctx, g); err != nil {
		return err
	}

	// Collect errors from resources that failed to register but were not fatal
	// (i.e., we continued processing to allow independent resources to proceed).
	nodeErrs := slices.Collect(e.failedNodes.Values())
	if len(nodeErrs) > 0 {
		return errors.Join(nodeErrs...)
	}

	// Process outputs (collect them into stackOutputs)
	for name, output := range e.config.Outputs {
		if err := e.processOutput(ctx, name, output); err != nil {
			return fmt.Errorf("processing output %s: %w", name, err)
		}
	}

	// Register stack outputs
	if err := e.registerStackOutputs(ctx); err != nil {
		return fmt.Errorf("registering stack outputs: %w", err)
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
func (e *Engine) processVariable(_ context.Context, node *graph.Node) error {
	v := node.Variable
	if v == nil {
		return fmt.Errorf("variable node missing Variable field")
	}

	// Module variable: evaluate input expression in parent context, store in each instance context.
	if node.ModuleInfo != nil {
		return e.processModuleVariable(node)
	}

	varName := node.Key[4:] // Remove "var." prefix
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
		// Check Pulumi stack config with project prefix
		configKey := e.projectName + ":" + varName
		if configVal, ok := e.pulumiConfig[configKey]; ok {
			val = cty.StringVal(configVal)
			valueSource = "config"
			// Check if it's a secret
			for _, secretKey := range e.configSecretKeys {
				if secretKey == configKey || secretKey == varName {
					isSecret = true
					break
				}
			}
		} else if configVal, ok := e.pulumiConfig[varName]; ok {
			// Also check without project prefix
			val = cty.StringVal(configVal)
			valueSource = "config"
			if slices.Contains(e.configSecretKeys, varName) {
				isSecret = true
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

	// Type conversion if value came from string source (env/config)
	if valueSource == "environment" || valueSource == "config" {
		if v.TypeConstraint != cty.NilType && v.TypeConstraint != cty.DynamicPseudoType {
			converted, err := convertStringToType(val.AsString(), v.TypeConstraint)
			if err != nil {
				return fmt.Errorf("variable %q: %w", varName, err)
			}
			val = converted
		} else {
			// No type constraint: try JSON parsing for structured values.
			if parsed, err := parseJSONAuto(val.AsString()); err == nil {
				val = parsed
			}
		}
	}

	// Fill in optional()-attribute defaults before sensitive marking.
	if v.TypeDefaults != nil && !val.IsNull() && val.IsKnown() {
		val = v.TypeDefaults.Apply(val)
	}

	if valueSource != "environment" && valueSource != "config" &&
		v.TypeConstraint != cty.NilType && v.TypeConstraint != cty.DynamicPseudoType &&
		!val.IsNull() && val.IsKnown() {
		if converted, err := ctyconvert.Convert(val, v.TypeConstraint); err == nil {
			val = converted
		}
	}

	// Handle sensitive marking
	if v.Sensitive || isSecret {
		val = val.Mark(eval.SensitiveMark)
	}

	// Store in eval context (needed for validation which may reference var.<name>)
	e.evaluator.Context().SetVariable(varName, val)

	// Run validations
	for i, validation := range v.Validations {
		// Evaluate condition
		condVal, diags := e.evaluator.EvaluateExpression(validation.Condition)
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
			// Get error message
			errMsgVal, diags := e.evaluator.EvaluateExpression(validation.ErrorMessage)
			var errMsg string
			if diags.HasErrors() || errMsgVal.Type() != cty.String {
				errMsg = "validation failed"
			} else {
				errMsg = errMsgVal.AsString()
			}
			return fmt.Errorf("validation failed for variable %q: %s", varName, errMsg)
		}
	}

	return nil
}

// convertStringToType converts a string value to the specified cty type.
func convertStringToType(s string, targetType cty.Type) (cty.Value, error) {
	switch {
	case targetType == cty.String:
		return cty.StringVal(s), nil
	case targetType == cty.Number:
		// Parse as number
		var f float64
		if _, err := fmt.Sscanf(s, "%f", &f); err != nil {
			return cty.NilVal, fmt.Errorf("cannot convert %q to number: %w", s, err)
		}
		return cty.NumberFloatVal(f), nil
	case targetType == cty.Bool:
		switch strings.ToLower(s) {
		case "true", "1", "yes", "on":
			return cty.True, nil
		case "false", "0", "no", "off":
			return cty.False, nil
		default:
			return cty.NilVal, fmt.Errorf("cannot convert %q to bool", s)
		}
	case targetType.IsListType() || targetType.IsTupleType() || targetType.IsSetType():
		// For complex types, try JSON parsing
		return parseJSONValue(s, targetType)
	case targetType.IsMapType() || targetType.IsObjectType():
		return parseJSONValue(s, targetType)
	default:
		// For other types, try to use it as-is
		return cty.StringVal(s), nil
	}
}

// parseJSONValue parses a JSON string into a cty value.
func parseJSONValue(s string, targetType cty.Type) (cty.Value, error) {
	// Use cty's built-in JSON unmarshaling
	val, err := json.Unmarshal([]byte(s), targetType)
	if err != nil {
		return cty.NilVal, fmt.Errorf("cannot parse JSON value: %w", err)
	}
	return val, nil
}

// parseJSONAuto parses a JSON string into a cty value, automatically inferring the type.
// This uses cty's jsondecode function, which handles plain JSON (objects, arrays, strings, etc.)
// without requiring a type descriptor.
func parseJSONAuto(s string) (cty.Value, error) {
	result, err := stdlib.JSONDecodeFunc.Call([]cty.Value{cty.StringVal(s)})
	if err != nil {
		return cty.NilVal, err
	}
	return result, nil
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
			return e.registerProviderInContext(ctx, node, provider, inst.EvalCtx, inst.URN, inst)
		})
	}

	return e.registerProviderInContext(ctx, node, provider, e.evaluator.Context(), e.stackURN, nil)
}

// resolvePassThroughProvider looks up a provider passed into a module via
// `providers = { <localKey> = <parentExpr> }` and returns the resolved
// URN::ID, or "" when the resource isn't in a module, there's no entry for
// localKey, or the parent expression doesn't yield a provider reference.
func (e *Engine) resolvePassThroughProvider(modInfo *graph.ModuleInfo, localKey string) string {
	if modInfo == nil || modInfo.Module == nil || localKey == "" {
		return ""
	}
	passExpr, ok := modInfo.Module.Providers[localKey]
	if !ok {
		return ""
	}
	parentCtx := e.parentEvalContext(modInfo)
	if parentCtx == nil {
		return ""
	}
	val, diags := eval.NewEvaluator(parentCtx).EvaluateExpression(passExpr)
	if diags.HasErrors() {
		return ""
	}
	ref, err := providerRefFromCty(val)
	if err != nil {
		return ""
	}
	return ref
}

// parentEvalContext returns the eval.Context of the enclosing module
// instance (or the root context when modInfo's parent is root).
func (e *Engine) parentEvalContext(modInfo *graph.ModuleInfo) *eval.Context {
	if modInfo.ParentPrefix() == "" {
		return e.evaluator.Context()
	}
	parentInsts, ok := e.moduleInstances.Get(modInfo.ParentPrefix())
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
	evalCtx *eval.Context, parentURN string, modInst *moduleInstance,
) error {
	typeToken := "pulumi:providers:" + provider.Name

	hclCtx := evalCtx.HCLContext()

	// Schema-aware eval is needed so schema.Property.Secret marks survive.
	pkg, perr := packages.ResolvePackage(ctx, e.pkgLoader, e.knownProviders(), "pulumi_providers_"+provider.Name)
	if perr != nil {
		return fmt.Errorf("resolving provider package %s: %w", provider.Name, perr)
	}
	resSchema, perr := pkg.Provider()
	if perr != nil {
		return fmt.Errorf("resolving provider schema for %s: %w", provider.Name, perr)
	}

	providerMapping := e.providerConfigBodyMapping(ctx, provider.Name)
	inputsMap, diags := transform.EvalResourceWithSchema(provider.Config, resSchema, providerMapping,
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
	if modInst != nil {
		modInstanceName := modInst.Path.LogicalName()
		logicalName = modInstanceName + "-" + logicalName
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
		PackageRef: e.packageRefs[provider.Name],
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
	outputObj["urn"] = cty.StringVal(resp.URN)

	e.resourceOutputs.Set(node.Key, eval.MarkOutputLeaves(cty.ObjectVal(outputObj), eval.DepMark(resp.URN)))

	// Top-level un-aliased provider blocks become the default provider for
	// resources of the same package that don't set `provider` explicitly.
	if provider.Alias == "" && node.ModuleInfo == nil && providerID != "" {
		e.defaultProviders.Set(provider.Name, resp.URN+"::"+providerID)
	}

	markedProviderOutputs := eval.MarkOutputLeaves(cty.ObjectVal(outputObj), eval.DepMark(resp.URN))
	if node.ModuleInfo != nil {
		// Strip prefix for module-internal references
		bareKey := strings.TrimPrefix(node.Key, node.ModuleInfo.Prefix())
		evalCtx.SetResource(bareKey, markedProviderOutputs)
	} else {
		evalCtx.SetResource(node.Key, markedProviderOutputs)
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
	evalCtx *eval.Context, parentURN string, modInst *moduleInstance,
) error {
	resSchema, err := e.resolveResource(ctx, res.Type)
	if err != nil {
		if diag := unknownTokenDiag("resource", res.TypeRange, err); diag != err {
			return diag
		}
		return fmt.Errorf("resolving resource type %s: %w", res.Type, err)
	}

	tempEvaluator := eval.NewEvaluator(evalCtx)

	expander := graph.NewResourceExpander()

	if res.Count != nil {
		count, isBool, diags := tempEvaluator.EvaluateCount(res.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating count: %s", diags.Error())
		}
		if isBool {
			expander.SetBoolCount(node.Key, count)
		} else {
			expander.SetCount(node.Key, count)
		}
	}

	if res.ForEach != nil {
		forEach, diags := tempEvaluator.EvaluateForEach(res.ForEach)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating for_each: %s", diags.Error())
		}
		expander.SetForEach(node.Key, forEach)
	}

	result := expander.Expand(node)

	for _, instance := range result.Instances {
		if e.hasFailedDependency(res) {
			e.failedNodes.Set(instance.Key, fmt.Errorf("skipped: dependency failed"))
			continue
		}
		if err := e.registerResourceInstanceInContext(
			ctx, node, res, resSchema, instance, evalCtx, parentURN, modInst,
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
		evalCtx.SetResource(baseKey, empty)
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
	parentURN string,
	modInst *moduleInstance,
) error {
	if instance.Index != nil {
		evalCtx.SetCount(*instance.Index)
		defer evalCtx.ClearCount()
	}
	if instance.EachKey != nil && instance.EachValue != nil {
		evalCtx.SetEach(*instance.EachKey, *instance.EachValue)
		defer evalCtx.ClearEach()
	}

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

	resourceMapping := e.resourceBodyMapping(ctx, res.Type)
	resourceInputs, diags := transform.EvalResourceWithSchema(res.Config, resSchema, resourceMapping,
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

	opts, err := e.buildResourceOptionsInContext(res, instance, evalCtx, parentURN, node.ModuleInfo)
	if err != nil {
		return err
	}
	opts.Custom = !resSchema.IsComponent
	opts.Remote = resSchema.IsComponent
	opts.PropertyDependencies = dependsOn
	for _, deps := range dependsOn {
		for _, dep := range deps {
			if !slices.Contains(opts.DependsOn, dep) {
				opts.DependsOn = append(opts.DependsOn, dep)
			}
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

	opts.PackageRef = e.packageRefForType(res.Type)

	if len(res.Preconditions) > 0 {
		if err := e.bindPreconditionHooks(ctx, res, instance, evalCtx, opts); err != nil {
			return err
		}
	}

	if len(res.Postconditions) > 0 {
		if err := e.bindPostconditionHooks(ctx, res, resSchema, resourceMapping, instance, evalCtx, opts); err != nil {
			return err
		}
	}

	if len(res.Provisioners) > 0 {
		if err := e.bindProvisionerHooks(ctx, res, resSchema, resourceMapping, instance, evalCtx, opts); err != nil {
			return err
		}
	}

	resourceName := e.extractModuleResourceName(res.Name, instance.Key, node.ModuleInfo, modInst)

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
	if e.dryRun && id == "" {
		outputObj["id"] = cty.UnknownVal(cty.String)
	} else {
		outputObj["id"] = cty.StringVal(id)
	}
	outputObj["urn"] = cty.StringVal(urn).Mark(eval.SyntheticMark)

	markedOutputs := eval.MarkOutputLeaves(cty.ObjectVal(outputObj), eval.DepMark(urn))

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
		evalCtx.SetCountResource(baseKey, *instance.Index, markedOutputs)
	} else if instance.EachKey != nil {
		baseKey := instance.OriginalKey
		if node.ModuleInfo != nil {
			baseKey = strings.TrimPrefix(baseKey, node.ModuleInfo.Prefix())
		}
		evalCtx.SetEachResource(baseKey, instance.EachKey.AsString(), markedOutputs)
	} else if node.ModuleInfo != nil {
		bareKey := strings.TrimPrefix(instance.Key, node.ModuleInfo.Prefix())
		evalCtx.SetResource(bareKey, markedOutputs)
	} else {
		evalCtx.SetResource(instance.Key, markedOutputs)
	}

	return nil
}

// buildResourceOptionsInContext builds resource options using the provided eval context and parent URN.
func (e *Engine) buildResourceOptionsInContext(
	res *ast.Resource, instance *graph.ExpandedResource,
	evalCtx *eval.Context, parentURN string,
	modInfo *graph.ModuleInfo,
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
		// ignore_changes maps to ignoreChanges
		for _, ic := range res.Lifecycle.IgnoreChanges {
			// ignore_changes can be relative traversals (just property names like "tags")
			// or absolute traversals. FormatTraversalForIgnoreChanges handles both.
			icStr := formatTraversalForIgnoreChanges(ic)
			if icStr != "" {
				opts.IgnoreChanges = append(opts.IgnoreChanges, icStr)
			}
		}
		if res.Lifecycle.IgnoreAllChanges {
			opts.IgnoreChanges = []string{"*"}
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
	opts.DeleteBeforeReplaceDef = true
	opts.DeleteBeforeReplace = res.Lifecycle == nil || res.Lifecycle.CreateBeforeDestroy == nil || !*res.Lifecycle.CreateBeforeDestroy

	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
				if urn := ctyAsString(outputs.GetAttr("urn")); urn != "" {
					opts.Parent = urn
				}
			}
		}
	}

	if res.Provider != nil {
		// `provider = X.alias` (or `provider = X`). For an in-module
		// resource, the parent module call may have mapped this local
		// reference to a parent-scope provider via
		// `providers = { X.alias = ... }` — that wins over any local
		// definition. Fall back to evaluating the expression in the
		// resource's own scope, which is what TF does when there is no
		// pass-through.
		if ref := e.resolvePassThroughProvider(modInfo, providerExprKey(res.Provider)); ref != "" {
			opts.Provider = ref
		} else {
			val, valDiags := eval.NewEvaluator(evalCtx).EvaluateExpression(res.Provider)
			if valDiags.HasErrors() {
				return nil, fmt.Errorf("evaluating provider for %s.%s: %s", res.Type, res.Name, valDiags.Error())
			}
			ref, err := providerRefFromCty(val)
			if err != nil {
				return nil, fmt.Errorf("resolving provider for %s.%s: %w", res.Type, res.Name, err)
			}
			opts.Provider = ref
		}
	} else if modInfo != nil {
		// Implicit default in a module: try a pass-through entry, then
		// the in-module `provider "<pkg>" {}` block (registered earlier
		// at key resPrefix+pkg in resourceOutputs). If neither exists,
		// fall through to Pulumi's engine default.
		pkg := packageNameFromResourceType(res.Type)
		if ref := e.resolvePassThroughProvider(modInfo, pkg); ref != "" {
			opts.Provider = ref
		} else if outputs, ok := e.resourceOutputs.Get(resPrefix + pkg); ok {
			if ref, err := providerRefFromCty(outputs); err == nil {
				opts.Provider = ref
			}
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
			if diags.HasErrors() || val.Type() != cty.String {
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
	movedAliases := e.resolveMovedAliases(res)
	opts.Aliases = append(opts.Aliases, movedAliases...)

	// Handle aliases attribute
	if res.Aliases != nil {
		aliases, err := e.evaluateAliases(res.Aliases)
		if err == nil {
			opts.Aliases = append(opts.Aliases, aliases...)
		}
	}

	// Handle import blocks - resolve import ID from import blocks that target this resource
	opts.ImportId = e.resolveImportId(res)

	hclCtx := evalCtx.HCLContext()

	if res.AdditionalSecretOutputs != nil {
		secretOutputsVal, diags := res.AdditionalSecretOutputs.Value(hclCtx)
		if !diags.HasErrors() && (secretOutputsVal.Type().IsListType() || secretOutputsVal.Type().IsTupleType()) {
			it := secretOutputsVal.ElementIterator()
			for it.Next() {
				_, elem := it.Element()
				if elem.Type() == cty.String {
					opts.AdditionalSecretOutputs = append(opts.AdditionalSecretOutputs, elem.AsString())
				}
			}
		}
	}

	if res.RetainOnDelete != nil {
		val, diags := res.RetainOnDelete.Value(hclCtx)
		if !diags.HasErrors() && val.Type() == cty.Bool {
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

	// Handle hide_diffs - property paths (already in camelCase)
	opts.HideDiffs = append(opts.HideDiffs, res.HideDiff...)

	// Handle replace_on_changes - property paths (already in camelCase)
	opts.ReplaceOnChanges = append(opts.ReplaceOnChanges, res.ReplaceOnChanges...)

	// `lifecycle { replace_triggered_by = [a, b, ...] }` evaluates each
	// expression and feeds the result to RegisterResource as the
	// ReplacementTrigger property value: any element flipping triggers a
	// replacement. A single-element list is unwrapped to a scalar so the
	// trigger value round-trips with Pulumi's scalar `replacementTrigger`.
	if res.Lifecycle != nil && len(res.Lifecycle.ReplaceTriggeredBy) > 0 {
		vals := make([]cty.Value, 0, len(res.Lifecycle.ReplaceTriggeredBy))
		for _, expr := range res.Lifecycle.ReplaceTriggeredBy {
			val, diags := expr.Value(hclCtx)
			if diags.HasErrors() {
				return nil, fmt.Errorf("evaluating replace_triggered_by on %q: %s",
					res.Type+"."+res.Name, diags.Error())
			}
			vals = append(vals, val)
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
		if !diags.HasErrors() && (val.Type().IsObjectType() || val.Type().IsMapType()) {
			mappings := make(map[string]string)
			for k, v := range val.AsValueMap() {
				if v.Type() == cty.String {
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
		if !diags.HasErrors() && val.Type() == cty.String {
			opts.Version = val.AsString()
		}
	}

	if res.PluginDownloadURL != nil && !strings.HasPrefix(res.Type, "pulumi_providers_") {
		val, diags := res.PluginDownloadURL.Value(hclCtx)
		if !diags.HasErrors() && val.Type() == cty.String {
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

// resolveMovedAliases finds any moved blocks that target this resource and returns
// the source addresses as aliases.
func (e *Engine) resolveMovedAliases(res *ast.Resource) []Alias {
	var aliases []Alias
	resourceAddr := res.Type + "." + res.Name

	for _, moved := range e.config.Moved {
		// Check if this moved block targets the current resource
		toAddr := graph.FormatTraversal(moved.To)
		if toAddr == resourceAddr {
			// Convert the "from" address to a URN-style alias
			fromAddr := graph.FormatTraversal(moved.From)
			if fromAddr != "" {
				// For Pulumi aliases, we use the resource name from the "from" address
				// The alias tells Pulumi this resource may have been known by a different name
				aliases = append(aliases, Alias{Spec: &AliasSpec{Name: fromAddr}})
			}
		}
	}

	return aliases
}

// resolveImportId finds any import blocks that target this resource and returns
// the import ID.
func (e *Engine) resolveImportId(res *ast.Resource) string {
	resourceAddr := res.Type + "." + res.Name

	for _, imp := range e.config.Imports {
		// Check if this import block targets the current resource
		toAddr := graph.FormatTraversal(imp.To)
		if toAddr == resourceAddr {
			return imp.Id
		}
	}

	return ""
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

func (e *Engine) knownProviders() []string {
	if e.config.Terraform == nil {
		return nil
	}
	providers := make([]string, 0, len(e.config.Terraform.RequiredProviders))
	for name := range e.config.Terraform.RequiredProviders {
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
	// Check resource body expressions.
	if res.Config != nil {
		attrs, _ := res.Config.JustAttributes()
		for _, attr := range attrs {
			for _, depKey := range eval.ExtractDependencies(attr.Expr) {
				if _, failed := e.failedNodes.Get(depKey); failed {
					return true
				}
			}
		}
	}
	return false
}

// buildResourceName builds the Pulumi resource name from the logical name and instance key.
// Single instances use the logical name as-is. Count instances get a "-N" suffix.
// ForEach instances get a "-key" suffix.
func buildResourceName(logicalName, instanceKey string) string {
	_, index, eachKey := graph.ParseInstanceKey(instanceKey)

	if index != nil {
		return fmt.Sprintf("%s-%d", logicalName, *index)
	}
	if eachKey != nil {
		return fmt.Sprintf("%s-%s", logicalName, *eachKey)
	}
	return logicalName
}

// extractModuleResourceName computes the Pulumi resource name for a resource inside a module.
// Resources inside a component are prefixed with the component instance name.
// For example, resource "res" inside component "comp" becomes "comp-res",
// and inside "comp[0]" becomes "comp[0]-res".
func (*Engine) extractModuleResourceName(
	logicalName, instanceKey string, modInfo *graph.ModuleInfo, modInst *moduleInstance,
) string {
	if modInfo == nil || modInst == nil {
		return buildResourceName(logicalName, instanceKey)
	}

	// Strip the module prefix to get the bare instance key (e.g., "simple_resource.name").
	bareKey := strings.TrimPrefix(instanceKey, modInfo.Prefix())
	bareResourceName := buildResourceName(logicalName, bareKey)

	// Extract the module instance name (e.g., "many" or "many[0]").
	modInstanceName := modInst.Path.LogicalName()

	return modInstanceName + "-" + bareResourceName
}

// formatTraversalForIgnoreChanges formats a traversal for ignore_changes.
// Handles both relative traversals (just "tags") and absolute ones.
func formatTraversalForIgnoreChanges(traversal hcl.Traversal) string {
	if len(traversal) == 0 {
		return ""
	}

	var buf strings.Builder
	for i, step := range traversal {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			buf.WriteString(s.Name)
		case hcl.TraverseAttr:
			if i > 0 {
				buf.WriteByte('.')
			}
			buf.WriteString(s.Name)
		case hcl.TraverseIndex:
			// Index traversals use bracket notation without a leading dot.
			key := s.Key
			if key.Type() == cty.String {
				fmt.Fprintf(&buf, "[%q]", key.AsString())
			} else if key.Type() == cty.Number {
				bf := key.AsBigFloat()
				if i64, acc := bf.Int64(); acc == 0 {
					fmt.Fprintf(&buf, "[%d]", i64)
				}
			}
		}
	}

	return buf.String()
}

// ResourceOptions contains resource registration options.
type ResourceOptions struct {
	Custom                  bool
	Remote                  bool
	DependsOn               []string
	PropertyDependencies    map[string][]string
	Protect                 bool
	IgnoreChanges           []string
	Aliases                 []Alias
	Provider                string
	Providers               map[string]string // Map from package name to provider reference (urn::id)
	Parent                  string
	DeleteBeforeReplace     bool
	DeleteBeforeReplaceDef  bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts          *CustomTimeouts
	ImportId                string
	AdditionalSecretOutputs []string
	RetainOnDelete          *bool
	DeletedWith             string         // URN of the resource that, when deleted, causes this resource to be deleted
	ReplaceWith             []string       // URNs of resources whose replacement triggers replacement of this resource
	HideDiffs               []string       // Property paths whose diffs should not be displayed
	ReplaceOnChanges        []string       // Property paths that if changed should force a replacement
	ReplacementTrigger      property.Value // Value whose change triggers replacement
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
) (string, string, property.Map, error) {
	inputs = lowerTerraformDataInputs(tfType, inputs, opts)

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
	funcSchema, err := e.resolveFunction(ctx, ds.Type)
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

	if ds.Count != nil {
		count, isBool, diags := tempEvaluator.EvaluateCount(ds.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating count: %s", diags.Error())
		}
		if isBool {
			expander.SetBoolCount(node.Key, count)
		} else {
			expander.SetCount(node.Key, count)
		}
	}

	if ds.ForEach != nil {
		forEach, diags := tempEvaluator.EvaluateForEach(ds.ForEach)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating for_each: %s", diags.Error())
		}
		expander.SetForEach(node.Key, forEach)
	}

	result := expander.Expand(node)

	var tupleOutputs []cty.Value
	eachOutputs := make(map[string]cty.Value)
	isForEach := ds.ForEach != nil

	for _, instance := range result.Instances {
		if instance.Index != nil {
			evalCtx.SetCount(*instance.Index)
		}
		if instance.EachKey != nil && instance.EachValue != nil {
			evalCtx.SetEach(*instance.EachKey, *instance.EachValue)
		}

		ctyOut, invokeErr := e.invokeDataSourceOnce(ctx, node, ds, funcSchema, evalCtx)

		if instance.Index != nil {
			evalCtx.ClearCount()
		}
		if instance.EachKey != nil {
			evalCtx.ClearEach()
		}

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

	var depMarks []any
	seen := make(map[string]bool)
	addURN := func(urn string) {
		if urn == "" || seen[urn] {
			return
		}
		seen[urn] = true
		depMarks = append(depMarks, eval.DepMark(urn))
	}

	dataSourceMapping := e.dataSourceBodyMapping(ctx, ds.Type)
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
		if ref := e.resolvePassThroughProvider(node.ModuleInfo, providerExprKey(ds.Provider)); ref != "" {
			invokeReq.Provider = ref
		} else {
			val, valDiags := eval.NewEvaluator(evalCtx).EvaluateExpression(ds.Provider)
			if valDiags.HasErrors() {
				return cty.NilVal, fmt.Errorf("evaluating provider for data %s.%s: %s", ds.Type, ds.Name, valDiags.Error())
			}
			ref, err := providerRefFromCty(val)
			if err != nil {
				return cty.NilVal, fmt.Errorf("resolving provider for data %s.%s: %w", ds.Type, ds.Name, err)
			}
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

	for _, m := range depMarks {
		ctyOutputs = eval.MarkOutputLeaves(ctyOutputs, m)
	}

	return ctyOutputs, nil
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
			resSchema, err = e.resolveResource(ctx, res.Type)
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
			providerToken := "pulumi_providers_" + matched.Name
			resType = providerToken
			pkg, err := packages.ResolvePackage(ctx, e.pkgLoader, e.knownProviders(), providerToken)
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

// invokeFunction invokes a Pulumi function (data source).
func (e *Engine) invokeFunction(ctx context.Context, tfType string, req InvokeRequest) (property.Map, error) {
	req, defaults, err := lowerRemoteStateInvoke(tfType, req)
	if err != nil {
		return property.Map{}, err
	}

	if e.resmon == nil { // TODO: Remove this check
		// No resource monitor - return empty outputs for testing
		return property.Map{}, nil
	}

	// Invoke the function
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
		resType, ok := p.Type.(*schema.ResourceType)
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

// processModule processes a module call.
// Terraform modules map to Pulumi component resources. The module's resources
// become children of the component, and module outputs are collected for references.
// moduleLoaderAdapter adapts modules.Loader to graph.ModuleLoader.
type moduleLoaderAdapter struct {
	loader *modules.Loader
}

func (a *moduleLoaderAdapter) LoadModule(source, version, workDir string) (*graph.LoadedModule, error) {
	loaded, err := a.loader.LoadModule(source, version, workDir)
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
	instances, ok := e.moduleInstances.Get(node.ModuleInfo.Prefix())
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
	varName := strings.TrimPrefix(node.Key, modInfo.Prefix()+"var.")

	moduleInputAttrs, _ := modInfo.Module.Config.JustAttributes()
	inputAttr, hasInput := moduleInputAttrs[varName]

	// Determine the parent evaluation context. For root-level modules this is
	// the root evaluator context; for nested modules it is the enclosing module
	// instance's context so that expressions like var.name resolve correctly.
	parentEvalCtx := e.evaluator.Context()
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPrefix())
		if ok && len(parentInstances) > 0 {
			parentEvalCtx = parentInstances[0].EvalCtx
		}
	}

	return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		var val cty.Value

		if hasInput {
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
		if v.TypeDefaults != nil && !val.IsNull() && val.IsKnown() {
			val = v.TypeDefaults.Apply(val)
		}

		// Coerce the value to match the variable's type constraint.
		if v.TypeConstraint != cty.NilType && !val.IsNull() && val.IsKnown() {
			if converted, err := ctyconvert.Convert(val, v.TypeConstraint); err == nil {
				val = converted
			}
		}

		if v.Sensitive {
			val = val.Mark("sensitive")
		}

		inst.EvalCtx.SetVariable(varName, val)
		return nil
	})
}

// processModuleInit processes a module init node: registers component resources and creates instances.
func (e *Engine) processModuleInit(ctx context.Context, node *graph.Node) error {
	modInfo := node.ModuleInfo
	mod := modInfo.Module

	componentType := fmt.Sprintf("components:index:%s", componentTypeName(modInfo.SourcePath))

	// For simple (non-counted) modules, create a single instance.
	// For count/for_each, evaluate and create multiple instances.

	parentURN := e.stackURN
	parentEvalCtx := e.evaluator.Context()

	// If this is a nested module, look up the parent instance URN. When the
	// parent has zero instances (count=0 / for_each empty) the entire inner
	// subtree must be skipped — registering an empty instances slice lets
	// downstream per-instance work (vars, locals, nested modules, resources)
	// loop zero times instead of falling back to the root context.
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPrefix())
		if !ok || len(parentInstances) == 0 {
			e.moduleInstances.Set(modInfo.Prefix(), nil)
			return nil
		}
		parentURN = parentInstances[0].URN
		parentEvalCtx = parentInstances[0].EvalCtx
	}

	// Load the child module to get variable type constraints for input coercion.
	loaderWorkDir := modInfo.ParentSourcePath
	if loaderWorkDir == "" {
		loaderWorkDir = e.workDir
	}
	childMod, err := e.moduleLoader.LoadModule(mod.Source, mod.Version, loaderWorkDir)
	if err != nil {
		return fmt.Errorf("loading module %s for input types: %w", mod.Source, err)
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

	// No count/for_each: single instance.
	if mod.Count == nil && mod.ForEach == nil {
		instPath := instancePath(modInfo, nil, nil)
		componentOpts := &ResourceOptions{Parent: parentURN}
		componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instPath.LogicalName(), property.NewMap(inputs), componentOpts)
		if err != nil {
			return fmt.Errorf("registering module component: %w", err)
		}

		instCtx := eval.NewContext(
			modInfo.SourcePath, e.workDir, e.workDir,
			e.stackName, e.projectName, e.organization,
		)

		e.moduleInstances.Set(modInfo.Prefix(), []*moduleInstance{{
			Path:    instPath,
			EvalCtx: instCtx,
			URN:     componentURN,
			Outputs: make(map[string]cty.Value),
		}})
		return nil
	}

	if mod.Count != nil {
		count, _, diags := eval.NewEvaluator(parentEvalCtx).EvaluateCount(mod.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module count: %s", diags.Error())
		}
		var instances []*moduleInstance
		for idx := range count {
			instPath := instancePath(modInfo, &idx, nil)
			componentOpts := &ResourceOptions{Parent: parentURN}
			componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instPath.LogicalName(), property.NewMap(inputs), componentOpts)
			if err != nil {
				return fmt.Errorf("registering module component %s: %w", instPath.String(), err)
			}
			instCtx := eval.NewContext(
				modInfo.SourcePath, e.workDir, e.workDir,
				e.stackName, e.projectName, e.organization,
			)
			instCtx.SetCount(idx)
			instances = append(instances, &moduleInstance{
				Path:    instPath,
				EvalCtx: instCtx,
				URN:     componentURN,
				Index:   &idx,
				Outputs: make(map[string]cty.Value),
			})
		}
		e.moduleInstances.Set(modInfo.Prefix(), instances)
		return nil
	}

	forEach, diags := eval.NewEvaluator(parentEvalCtx).EvaluateForEach(mod.ForEach)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating module for_each: %s", diags.Error())
	}

	var instances []*moduleInstance
	for _, ks := range slices.Sorted(maps.Keys(forEach)) {
		k := cty.StringVal(ks)
		v := forEach[ks]
		instPath := instancePath(modInfo, nil, &k)
		componentOpts := &ResourceOptions{Parent: parentURN}
		componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instPath.LogicalName(), property.NewMap(inputs), componentOpts)
		if err != nil {
			return fmt.Errorf("registering module component %s: %w", instPath.String(), err)
		}
		instCtx := eval.NewContext(
			modInfo.SourcePath, e.workDir, e.workDir,
			e.stackName, e.projectName, e.organization,
		)
		instCtx.SetEach(k, v)
		instances = append(instances, &moduleInstance{
			Path:    instPath,
			EvalCtx: instCtx,
			URN:     componentURN,
			EachKey: &k,
			EachVal: &v,
			Outputs: make(map[string]cty.Value),
		})
	}
	e.moduleInstances.Set(modInfo.Prefix(), instances)
	return nil
}

// processModuleOutput evaluates a module output in each instance and stores it in the parent context.
func (e *Engine) processModuleOutput(_ context.Context, node *graph.Node) error {
	output := node.Output
	modInfo := node.ModuleInfo
	outputName := strings.TrimPrefix(node.Key, modInfo.Prefix()+"output.")
	mod := modInfo.Module
	isCounted := mod.Count != nil
	isForEach := mod.ForEach != nil

	err := e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		val, diags := output.Value.Value(inst.EvalCtx.HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module output %s: %s", outputName, diags.Error())
		}
		inst.mu.Lock()
		inst.Outputs[outputName] = val
		inst.mu.Unlock()
		return nil
	})
	if err != nil {
		return err
	}

	// Eagerly publish outputs to the parent context so other module variables
	// can reference them before the completion node runs. For nested modules,
	// the parent context is the enclosing module instance's eval context.
	parentCtx := e.evaluator.Context()
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPrefix())
		if ok && len(parentInstances) > 0 {
			parentCtx = parentInstances[0].EvalCtx
		}
	}
	instances, ok := e.moduleInstances.Get(modInfo.Prefix())
	if !ok {
		return nil
	}

	if !isCounted && !isForEach {
		if len(instances) == 1 {
			inst := instances[0]
			inst.mu.Lock()
			v, has := inst.Outputs[outputName]
			inst.mu.Unlock()
			if has {
				parentCtx.SetModuleOutput(modInfo.ModuleName(), outputName, v)
			}
		}
	} else if isCounted {
		// Rebuild the full tuple from all collected outputs so far.
		tupleVals := make([]cty.Value, len(instances))
		for i, inst := range instances {
			inst.mu.Lock()
			if len(inst.Outputs) > 0 {
				tupleVals[i] = cty.ObjectVal(inst.Outputs)
			} else {
				tupleVals[i] = cty.EmptyObjectVal
			}
			inst.mu.Unlock()
		}
		if len(tupleVals) > 0 {
			parentCtx.SetModule(modInfo.ModuleName(), cty.TupleVal(tupleVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName(), cty.EmptyTupleVal)
		}
	} else {
		// ForEach: rebuild the map.
		mapVals := make(map[string]cty.Value, len(instances))
		for _, inst := range instances {
			if inst.EachKey == nil {
				continue
			}
			keyStr := inst.EachKey.AsString()
			inst.mu.Lock()
			if len(inst.Outputs) > 0 {
				mapVals[keyStr] = cty.ObjectVal(inst.Outputs)
			} else {
				mapVals[keyStr] = cty.EmptyObjectVal
			}
			inst.mu.Unlock()
		}
		if len(mapVals) > 0 {
			parentCtx.SetModule(modInfo.ModuleName(), cty.ObjectVal(mapVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName(), cty.EmptyObjectVal)
		}
	}

	return nil
}

// processModuleComplete handles the module completion node: registers component outputs
// and assembles the full module value in the parent context.
func (e *Engine) processModuleComplete(ctx context.Context, node *graph.Node) error {
	modInfo := node.ModuleInfo
	if modInfo == nil {
		return fmt.Errorf("module completion node missing ModuleInfo")
	}

	instances, ok := e.moduleInstances.Get(modInfo.Prefix())
	if !ok {
		return fmt.Errorf("no module instances for prefix %q", modInfo.Prefix())
	}

	mod := modInfo.Module
	isCounted := mod.Count != nil
	isForEach := mod.ForEach != nil

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

	// Assemble module value in parent eval context. For nested modules, the
	// parent context is the enclosing module instance's eval context.
	parentCtx := e.evaluator.Context()
	if modInfo.ParentPrefix() != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPrefix())
		if ok && len(parentInstances) > 0 {
			parentCtx = parentInstances[0].EvalCtx
		}
	}

	if !isCounted && !isForEach {
		// Single instance: module.X is an object of outputs.
		if len(instances) == 1 {
			for k, v := range instances[0].Outputs {
				parentCtx.SetModuleOutput(modInfo.ModuleName(), k, v)
			}
		}
	} else if isCounted {
		// Counted: module.X is a tuple/list of output objects.
		tupleVals := make([]cty.Value, len(instances))
		for i, inst := range instances {
			if len(inst.Outputs) > 0 {
				tupleVals[i] = cty.ObjectVal(inst.Outputs)
			} else {
				tupleVals[i] = cty.EmptyObjectVal
			}
		}
		if len(tupleVals) > 0 {
			parentCtx.SetModule(modInfo.ModuleName(), cty.TupleVal(tupleVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName(), cty.EmptyTupleVal)
		}
	} else {
		// ForEach: module.X is a map of key → output object.
		mapVals := make(map[string]cty.Value, len(instances))
		for _, inst := range instances {
			if inst.EachKey == nil {
				continue
			}
			keyStr := inst.EachKey.AsString()
			if len(inst.Outputs) > 0 {
				mapVals[keyStr] = cty.ObjectVal(inst.Outputs)
			} else {
				mapVals[keyStr] = cty.EmptyObjectVal
			}
		}
		if len(mapVals) > 0 {
			parentCtx.SetModule(modInfo.ModuleName(), cty.ObjectVal(mapVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName(), cty.EmptyObjectVal)
		}
	}

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
) (string, string, property.Map, error) {
	if e.resmon == nil {
		urn := fmt.Sprintf("urn:pulumi:%s::%s::%s::%s",
			e.stackName, e.projectName, typeToken, name)
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
	// Evaluate the output value, intercepting can() calls.
	val, diags := e.evaluator.EvaluateExpression(output.Value)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating output value: %s", diags.Error())
	}

	// Convert to PropertyValue
	pv, err := transform.CtyToPropertyValue(val)
	if err != nil {
		return fmt.Errorf("converting output value: %w", err)
	}

	// Mark as secret if sensitive
	if output.Sensitive {
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
//   - an object carrying an `__ref` ResourceReference capsule (e.g., the result
//     of a `call.<resource>.<method>` that returns a provider).
func providerRefFromCty(val cty.Value) (string, error) {
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
	if val.Type().HasAttribute("__ref") {
		refVal, _ := val.GetAttr("__ref").Unmark()
		if refVal.Type() != eval.ResourceReferenceCapsuleType {
			return "", fmt.Errorf("provider value has __ref of unexpected type %s", refVal.Type().FriendlyName())
		}
		if refVal.IsNull() {
			return "", errors.New("provider value has null __ref")
		}
		ref := refVal.EncapsulatedValue().(*property.ResourceReference)
		id := ""
		if ref.ID.IsString() {
			id = ref.ID.AsString()
		}
		return string(ref.URN) + "::" + id, nil
	}
	if val.Type().HasAttribute("urn") && val.Type().HasAttribute("id") {
		urn := ctyAsString(val.GetAttr("urn"))
		id := ctyAsString(val.GetAttr("id"))
		if urn == "" || id == "" {
			return "", fmt.Errorf("provider value urn/id must be non-empty strings, got urn=%q id=%q", urn, id)
		}
		return urn + "::" + id, nil
	}
	return "", errors.New("provider value is not a resource reference")
}

// RunFromDirectory parses and executes an HCL program from a directory.
func RunFromDirectory(ctx context.Context, dir string, opts *EngineOptions) error {
	// Parse the configuration
	p := parser.NewParser()
	config, diags := p.ParseDirectory(dir)
	if diags.HasErrors() {
		return fmt.Errorf("parsing configuration: %s", diags.Error())
	}

	// Set the work dir if not specified
	if opts.WorkDir == "" {
		opts.WorkDir = dir
	}

	// Create and run the engine
	engine := NewEngine(ctx, config, opts)
	return engine.Run(ctx)
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
) error {
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	for i, rule := range res.Preconditions {
		rule, index := rule, i+1
		hookName := fmt.Sprintf("%s:precondition:%d", instance.Key, i)
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
) error {
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	dryRun := e.dryRun
	for i, rule := range res.Postconditions {
		rule, index := rule, i+1
		hookName := fmt.Sprintf("%s:postcondition:%d", instance.Key, i)
		callback := func(_ context.Context, args *ResourceHookArgs) error {
			return evaluatePostcondition(rule, hclSnapshot, args.NewOutputs, resSchema, mapping, dryRun, index, instance.Key)
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
	rule *ast.CheckRule, hclCtx *hcl.EvalContext, newOutputs property.Map,
	resSchema *schema.Resource, mapping *bridge.BodyMapping, dryRun bool, index int, resourceName string,
) error {
	outputObj, err := transform.ResourceOutputToCty(newOutputs, resSchema, mapping, dryRun)
	if err != nil {
		return fmt.Errorf("converting outputs for postcondition %d on %s: %w", index, resourceName, err)
	}
	selfCtx := hclCtx.NewChild()
	selfCtx.Variables = map[string]cty.Value{"self": cty.ObjectVal(outputObj)}
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
	if s := ctyAsString(msgVal); s != "" {
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
	if s := ctyAsString(msgVal); s != "" {
		msg = s
	}
	return fmt.Errorf("precondition for %s: %s", resourceName, msg)
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
func ctyAsString(v cty.Value) string {
	if v.IsMarked() {
		v, _ = v.Unmark()
	}
	if v.IsNull() || !v.IsKnown() || v.Type() != cty.String {
		return ""
	}
	return v.AsString()
}
