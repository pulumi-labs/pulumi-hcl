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
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/graph"
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
	Key     string               // e.g., "module.first" or "module.first[0]"
	EvalCtx *eval.Context        // per-instance evaluation context
	URN     string               // component URN
	Index   *int                 // count index (nil if not using count)
	EachKey *cty.Value           // for_each key (nil if not using for_each)
	EachVal *cty.Value           // for_each value (nil if not using for_each)
	Outputs map[string]cty.Value // collected output values
}

// inheritableOpts holds the resource options that child resources can inherit from their parent.
type inheritableOpts struct {
	Protect        *bool
	RetainOnDelete *bool
}

// Engine executes HCL programs against the Pulumi engine.
type Engine struct {
	// config is the parsed HCL configuration.
	config *ast.Config

	// evaluator handles expression evaluation.
	evaluator *eval.Evaluator

	// pkgLoader loads Pulumi package schemas.
	pkgLoader schema.ReferenceLoader

	// resmon is the resource monitor for registering resources.
	resmon ResourceMonitor

	// resourceOutputs maps resource keys to their output values.
	resourceOutputs *util.SyncMap[string, cty.Value]

	// resourceInheritableOpts maps resource keys to the options that children can inherit.
	resourceInheritableOpts *util.SyncMap[string, inheritableOpts]

	// dataSourceDependencies maps data source keys to their resource dependencies (URNs).
	dataSourceDependencies *util.SyncMap[string, []resource.URN]

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

	// Packages maps parameterized package alias to its descriptor.
	// The engine calls RegisterPackage on the resource monitor for each entry before running the program.
	Packages map[string]workspace.PackageDescriptor

	Parallel int
}

// NewEngine creates a new execution engine.
func NewEngine(config *ast.Config, opts *EngineOptions) *Engine {
	contract.Assertf(opts.SchemaLoader != nil, "EngineOptions.SchemaLoader cannot be nil")
	contract.Assertf(opts.WorkDir != "", "EngineOptions.WorkDir cannot be empty")
	contract.Assertf(opts.RootDir != "", "EngineOptions.RootDir cannot be empty")

	evalCtx := eval.NewContext(opts.WorkDir, opts.RootDir,
		opts.StackName, opts.ProjectName, opts.Organization)

	engine := &Engine{
		config:                  config,
		evaluator:               eval.NewEvaluator(evalCtx),
		pkgLoader:               opts.SchemaLoader,
		resmon:                  opts.ResourceMonitor,
		resourceOutputs:         util.NewSyncMap[string, cty.Value](),
		resourceInheritableOpts: util.NewSyncMap[string, inheritableOpts](),
		dataSourceDependencies:  util.NewSyncMap[string, []resource.URN](),
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
		moduleLoader:            modules.NewLoader(),
		moduleInstances:         util.NewSyncMap[string, []*moduleInstance](),
		parallel:                opts.Parallel,
		failedNodes:             util.NewSyncMap[string, error](),
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

	// If no value from env or config, use default
	if valueSource == "" {
		if v.Default != nil {
			var diags hcl.Diagnostics
			val, diags = e.evaluator.EvaluateExpression(v.Default)
			if diags.HasErrors() {
				return fmt.Errorf("evaluating variable default: %s", diags.Error())
			}
			valueSource = "default"
		} else if v.Nullable {
			// Variable is nullable and has no value - use null
			val = cty.NullVal(cty.DynamicPseudoType)
			valueSource = "null"
		} else {
			// Variable is required but no value provided
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

	// Handle sensitive marking
	if v.Sensitive || isSecret {
		val = val.Mark("sensitive")
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

		if condVal.Type() != cty.Bool {
			return fmt.Errorf("validation condition %d for variable %q must be boolean, got %s",
				i+1, varName, condVal.Type().FriendlyName())
		}

		if condVal.False() {
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
			localName := strings.TrimPrefix(node.Key, node.ModuleInfo.Prefix+"local.")
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

	if node.ModuleInfo != nil {
		return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
			return e.registerProviderInContext(ctx, node, provider, inst.EvalCtx, inst.URN, inst)
		})
	}

	return e.registerProviderInContext(ctx, node, provider, e.evaluator.Context(), e.stackURN, nil)
}

func (e *Engine) registerProviderInContext(
	ctx context.Context, node *graph.Node, provider *ast.Provider,
	evalCtx *eval.Context, parentURN string, modInst *moduleInstance,
) error {
	typeToken := "pulumi:providers:" + provider.Name

	attrs, _ := provider.Config.JustAttributes()
	inputs := make(map[string]property.Value)

	hclCtx := evalCtx.HCLContext()
	for name, attr := range attrs {
		if name == "alias" {
			continue
		}

		val, diags := attr.Expr.Value(hclCtx)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating provider attribute %s: %s", name, diags.Error())
		}

		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting provider attribute %s: %w", name, err)
		}

		inputs[name] = pv
	}

	logicalName := provider.Alias
	if logicalName == "" {
		logicalName = provider.Name
	}
	if modInst != nil {
		modInstanceName := extractResourceName(modInst.Key)
		logicalName = modInstanceName + "-" + logicalName
	}

	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:   typeToken,
		Name:   logicalName,
		Inputs: property.NewMap(inputs),
		Custom: true,
		Parent: parentURN,
	})
	if err != nil {
		return fmt.Errorf("registering provider %s: %w", node.Key, err)
	}

	providerID := resp.ID

	outputObj := make(map[string]cty.Value)
	outputObj["id"] = cty.StringVal(providerID)
	outputObj["urn"] = cty.StringVal(resp.URN)

	for k, v := range resp.Outputs.All {
		snakeKey := camelToSnake(string(k))
		outputObj[snakeKey] = transform.PropertyValueToCty(v)
	}

	e.resourceOutputs.Set(node.Key, cty.ObjectVal(outputObj))

	if node.ModuleInfo != nil {
		// Strip prefix for module-internal references
		bareKey := strings.TrimPrefix(node.Key, node.ModuleInfo.Prefix)
		evalCtx.SetResource(bareKey, cty.ObjectVal(outputObj))
	} else {
		evalCtx.SetResource(node.Key, cty.ObjectVal(outputObj))
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
	resSchema, err := packages.ResolveResource(ctx, e.pkgLoader, e.knownProviders(), res.Type)
	if err != nil {
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

	resourceInputs, diags := transform.EvalResourceWithSchema(res.Config, resSchema,
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

			for _, dep := range eval.ExtractDependencies(expr) {
				fullDep := dep
				if node.ModuleInfo != nil {
					fullDep = node.ModuleInfo.Prefix + dep
				}
				if resOutputs, ok := e.resourceOutputs.Get(fullDep); ok {
					if urnVal := resOutputs.GetAttr("urn"); urnVal.Type() == cty.String {
						addToDependsOn(string(propKey), urnVal.AsString())
					}
				}
				if dsKey, ok := strings.CutPrefix(fullDep, "data."); ok {
					if dsDeps, exists := e.dataSourceDependencies.Get(dsKey); exists {
						for _, urn := range dsDeps {
							addToDependsOn(string(propKey), string(urn))
						}
					}
				}
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
			if req, ok := e.config.Terraform.RequiredProviders[pkgName]; ok && req.Version != "" {
				opts.Version = ExtractSemverFromConstraint(req.Version)
			}
		}
	}

	if opts.PluginDownloadURL == "" && resSchema.PackageReference != nil {
		opts.PluginDownloadURL = resSchema.PackageReference.PluginDownloadURL()
	}

	opts.PackageRef = e.packageRefForType(res.Type)

	if len(res.Preconditions) > 0 {
		if err := e.evaluateCheckRules(res.Preconditions, instance.Key, "precondition"); err != nil {
			return err
		}
	}

	resourceName := e.extractModuleResourceName(instance.Key, node.ModuleInfo, modInst)

	urn, id, outputs, err := e.registerResource(ctx, resSchema.Token, resourceName, resourceInputs, opts)
	if err != nil {
		e.failedNodes.Set(instance.Key, fmt.Errorf("registering resource: %w", err))
		return nil
	}

	outputs = outputs.Delete("id", "urn")

	outputs, err = e.resolveResourceRefsInOutputs(ctx, outputs, resSchema)
	if err != nil {
		return fmt.Errorf("resolving resource references in outputs: %w", err)
	}

	outputObj, err := transform.ResourceOutputToCty(outputs, resSchema, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting resource outputs to HCL types: %w", err)
	}
	outputObj["id"] = cty.StringVal(id)
	outputObj["urn"] = cty.StringVal(urn)

	e.resourceOutputs.Set(instance.Key, cty.ObjectVal(outputObj))

	var iOpts inheritableOpts
	if opts.Protect {
		iOpts.Protect = new(true)
	}
	iOpts.RetainOnDelete = opts.RetainOnDelete
	e.resourceInheritableOpts.Set(instance.Key, iOpts)

	if node.ModuleInfo != nil {
		bareKey := strings.TrimPrefix(instance.Key, node.ModuleInfo.Prefix)
		evalCtx.SetResource(bareKey, cty.ObjectVal(outputObj))
	} else {
		evalCtx.SetResource(instance.Key, cty.ObjectVal(outputObj))
	}

	if len(res.Postconditions) > 0 {
		evalCtx.SetSelf(cty.ObjectVal(outputObj))
		defer evalCtx.ClearSelf()
		if err := e.evaluateCheckRules(res.Postconditions, instance.Key, "postcondition"); err != nil {
			return err
		}
	}

	if len(res.Provisioners) > 0 {
		if err := e.processProvisioners(ctx, res, urn, cty.ObjectVal(outputObj), instance.Key); err != nil {
			return fmt.Errorf("processing provisioners: %w", err)
		}
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
		resPrefix = modInfo.Prefix
	}

	for _, dep := range res.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
			urnVal := outputs.GetAttr("urn")
			if urnVal.Type() == cty.String {
				opts.DependsOn = append(opts.DependsOn, urnVal.AsString())
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
		// create_before_destroy controls replacement order:
		// - true: create new, then delete old (Pulumi's default behavior)
		// - false: delete old, then create new (Terraform's default behavior)
		// - nil/unset: use Pulumi's default (create-then-delete)
		//
		// Pulumi's deleteBeforeReplace is the inverse:
		// - true: delete old, then create new
		// - false: create new, then delete old (default)
		if res.Lifecycle.CreateBeforeDestroy != nil {
			if *res.Lifecycle.CreateBeforeDestroy {
				// Explicit true: create-then-delete (Pulumi default, but mark as explicitly set)
				opts.DeleteBeforeReplace = false
				opts.DeleteBeforeReplaceDef = true
			} else {
				// Explicit false: delete-then-create (Terraform default)
				opts.DeleteBeforeReplace = true
				opts.DeleteBeforeReplaceDef = true
			}
		}
		if len(res.Lifecycle.ReplaceTriggeredBy) > 0 {
			return nil, fmt.Errorf("lifecycle \"replace_triggered_by\" on resource %q is not supported by Pulumi HCL",
				res.Type+"."+res.Name)
		}
	}

	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(resPrefix + depKey); ok {
				urnVal := outputs.GetAttr("urn")
				if urnVal.Type() == cty.String {
					opts.Parent = urnVal.AsString()
				}
			}
		}
	}

	if res.Provider != nil {
		providerKey := res.Provider.Name
		if res.Provider.Alias != "" {
			providerKey = res.Provider.Name + "." + res.Provider.Alias
		}
		if providerOutputs, ok := e.resourceOutputs.Get(resPrefix + providerKey); ok {
			urnVal := providerOutputs.GetAttr("urn")
			idVal := providerOutputs.GetAttr("id")
			if urnVal.Type() == cty.String && idVal.Type() == cty.String {
				opts.Provider = urnVal.AsString() + "::" + idVal.AsString()
			}
		}
	}

	for _, traversal := range res.Providers {
		providerKey := graph.FormatTraversal(traversal)
		if providerKey == "" {
			continue
		}
		if providerOutputs, ok := e.resourceOutputs.Get(resPrefix + providerKey); ok {
			urnVal := providerOutputs.GetAttr("urn")
			idVal := providerOutputs.GetAttr("id")
			if urnVal.Type() == cty.String && idVal.Type() == cty.String {
				pkgName := packageNameFromResourceType(strings.SplitN(providerKey, ".", 2)[0])
				if opts.Providers == nil {
					opts.Providers = make(map[string]string)
				}
				opts.Providers[pkgName] = urnVal.AsString() + "::" + idVal.AsString()
			}
		}
	}

	// Handle timeouts
	if res.Timeouts != nil {
		ct := &CustomTimeouts{}
		hasTimeouts := false
		if res.Timeouts.Create != "" {
			if d, err := time.ParseDuration(res.Timeouts.Create); err == nil {
				ct.Create = d.Seconds()
				hasTimeouts = true
			}
		}
		if res.Timeouts.Update != "" {
			if d, err := time.ParseDuration(res.Timeouts.Update); err == nil {
				ct.Update = d.Seconds()
				hasTimeouts = true
			}
		}
		if res.Timeouts.Delete != "" {
			if d, err := time.ParseDuration(res.Timeouts.Delete); err == nil {
				ct.Delete = d.Seconds()
				hasTimeouts = true
			}
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
				urnVal := outputs.GetAttr("urn")
				if urnVal.Type() == cty.String {
					opts.DeletedWith = urnVal.AsString()
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
			urnVal := outputs.GetAttr("urn")
			if urnVal.Type() == cty.String {
				opts.ReplaceWith = append(opts.ReplaceWith, urnVal.AsString())
			}
		}
	}

	// Handle hide_diffs - property paths (already in camelCase)
	opts.HideDiffs = append(opts.HideDiffs, res.HideDiff...)

	// Handle replace_on_changes - property paths (already in camelCase)
	opts.ReplaceOnChanges = append(opts.ReplaceOnChanges, res.ReplaceOnChanges...)

	if res.ReplacementTrigger != nil {
		val, diags := res.ReplacementTrigger.Value(hclCtx)
		if !diags.HasErrors() {
			pv, err := transform.CtyToPropertyValue(val)
			if err == nil {
				opts.ReplacementTrigger = pv
			}
		}
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
			aliases = append(aliases, Alias{URN: elem.AsString()})
		} else if elem.Type().IsObjectType() {
			spec := &AliasSpec{}
			objType := elem.Type()
			if objType.HasAttribute("name") {
				if v := elem.GetAttr("name"); v.Type() == cty.String {
					spec.Name = v.AsString()
				}
			}
			if objType.HasAttribute("type") {
				if v := elem.GetAttr("type"); v.Type() == cty.String {
					spec.Type = v.AsString()
				}
			}
			if objType.HasAttribute("stack") {
				if v := elem.GetAttr("stack"); v.Type() == cty.String {
					spec.Stack = v.AsString()
				}
			}
			if objType.HasAttribute("project") {
				if v := elem.GetAttr("project"); v.Type() == cty.String {
					spec.Project = v.AsString()
				}
			}
			if objType.HasAttribute("parent_urn") {
				if v := elem.GetAttr("parent_urn"); v.Type() == cty.String {
					spec.ParentURN = v.AsString()
				}
			}
			if objType.HasAttribute("no_parent") {
				if v := elem.GetAttr("no_parent"); v.Type() == cty.Bool {
					spec.NoParent = v.True()
				}
			}
			aliases = append(aliases, Alias{Spec: spec})
		}
	}
	return aliases, nil
}

// extractResourceName extracts the resource name from an instance key.
// For example: "pulumi_stash.myStash" -> "myStash", "aws_instance.web[0]" -> "web[0]".
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

func ExtractSemverFromConstraint(constraint string) string {
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

// extractResourceName converts an instance key into a Pulumi resource name.
// Single instances use the logical name as-is. Count instances get a "-N" suffix.
// ForEach instances get a "-key" suffix.
func extractResourceName(key string) string {
	baseKey, index, eachKey := graph.ParseInstanceKey(key)

	// Strip the "type." prefix from the base key to get the logical name.
	if _, after, ok := strings.Cut(baseKey, "."); ok {
		baseKey = after
	}

	if index != nil {
		return fmt.Sprintf("%s-%d", baseKey, *index)
	}
	if eachKey != nil {
		return fmt.Sprintf("%s-%s", baseKey, *eachKey)
	}
	return baseKey
}

// extractModuleResourceName computes the Pulumi resource name for a resource inside a module.
// Resources inside a component are prefixed with the component instance name.
// For example, resource "res" inside component "comp" becomes "comp-res",
// and inside "comp[0]" becomes "comp[0]-res".
func (*Engine) extractModuleResourceName(
	instanceKey string, modInfo *graph.ModuleInfo, modInst *moduleInstance,
) string {
	if modInfo == nil || modInst == nil {
		return extractResourceName(instanceKey)
	}

	// Strip the module prefix to get the bare resource key (e.g., "simple_resource.name").
	bareKey := strings.TrimPrefix(instanceKey, modInfo.Prefix)
	bareResourceName := extractResourceName(bareKey)

	// Extract the module instance name (e.g., "many" or "many[0]").
	modInstanceName := extractResourceName(modInst.Key)

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
}

// registerResource registers a resource with the Pulumi engine.
func (e *Engine) registerResource(
	ctx context.Context,
	typeToken string,
	name string,
	inputs property.Map,
	opts *ResourceOptions,
) (string, string, property.Map, error) {
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

	// Resolve the data source type to a Pulumi function token
	funcSchema, err := packages.ResolveFunction(ctx, e.pkgLoader, e.knownProviders(), ds.Type)
	if err != nil {
		return fmt.Errorf("resolving data source type %s: %w", ds.Type, err)
	}

	var allDeps []resource.URN

	inputs, diags := transform.EvalFunctionWithSchema(ds.Config, funcSchema,
		func(propKey resource.PropertyKey, expr hcl.Expression, extraVars map[string]cty.Value) (cty.Value, hcl.Diagnostics) {
			var val cty.Value
			var diags hcl.Diagnostics
			if len(extraVars) > 0 {
				childCtx := e.evaluator.Context().HCLContext().NewChild()
				childCtx.Variables = extraVars
				val, diags = expr.Value(childCtx)
			} else {
				val, diags = e.evaluator.EvaluateExpression(expr)
			}
			if diags.HasErrors() {
				return val, diags
			}

			for _, dep := range eval.ExtractDependencies(expr) {
				if resOutputs, ok := e.resourceOutputs.Get(dep); ok {
					if urnVal := resOutputs.GetAttr("urn"); urnVal.Type() == cty.String {
						allDeps = append(allDeps, resource.URN(urnVal.AsString()))
					}
				}
				if dsKey, ok := strings.CutPrefix(dep, "data."); ok {
					if dsDeps, exists := e.dataSourceDependencies.Get(dsKey); exists {
						allDeps = append(allDeps, dsDeps...)
					}
				}
			}

			return val, diags
		})
	if diags.HasErrors() {
		return diags
	}

	invokeReq := InvokeRequest{
		Token:      funcSchema.Token,
		Args:       inputs,
		PackageRef: e.packageRefForType(ds.Type),
	}

	if ds.Provider != nil {
		providerKey := ds.Provider.Name
		if ds.Provider.Alias != "" {
			providerKey = ds.Provider.Name + "." + ds.Provider.Alias
		}
		if providerOutputs, ok := e.resourceOutputs.Get(providerKey); ok {
			urnVal := providerOutputs.GetAttr("urn")
			idVal := providerOutputs.GetAttr("id")
			if urnVal.Type() == cty.String && idVal.Type() == cty.String {
				invokeReq.Provider = urnVal.AsString() + "::" + idVal.AsString()
			}
		}
	}

	if ds.Version != nil {
		val, valDiags := ds.Version.Value(e.evaluator.Context().HCLContext())
		if !valDiags.HasErrors() && val.Type() == cty.String {
			invokeReq.Version = val.AsString()
		}
	}

	if ds.PluginDownloadURL != nil {
		val, valDiags := ds.PluginDownloadURL.Value(e.evaluator.Context().HCLContext())
		if !valDiags.HasErrors() && val.Type() == cty.String {
			invokeReq.PluginDownloadURL = val.AsString()
		}
	}

	for _, dep := range ds.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(depKey); ok {
			urnVal := outputs.GetAttr("urn")
			if urnVal.Type() == cty.String {
				allDeps = append(allDeps, resource.URN(urnVal.AsString()))
			}
		}
	}

	outputs, err := e.invokeFunction(ctx, invokeReq)
	if err != nil {
		return fmt.Errorf("invoking data source: %w", err)
	}

	ctyOutputs, err := transform.FunctionOutputToCty(outputs, funcSchema, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting function outputs to HCL types: %w", err)
	}

	// Store outputs for future references
	dsKey := node.Key[5:] // Remove "data." prefix
	e.evaluator.Context().SetDataSource(dsKey, ctyOutputs)

	// Store dependencies for this data source
	e.dataSourceDependencies.Set(dsKey, allDeps)

	return nil
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
			resSchema, err = packages.ResolveResource(ctx, e.pkgLoader, e.knownProviders(), res.Type)
			if err != nil {
				return fmt.Errorf("resolving resource type %s for call: %w", res.Type, err)
			}
			isProviderResource = strings.HasPrefix(res.Type, "pulumi_providers_")
			break
		}
	}

	if resKey == "" {
		// Try providers
		if _, exists := e.config.Providers[call.ResourceName]; exists {
			resKey = call.ResourceName
			var err error
			// Providers use their name as the key
			providerToken := "pulumi_providers_" + e.config.Providers[call.ResourceName].Name
			resType = providerToken
			resSchema, err = packages.ResolveResource(ctx, e.pkgLoader, e.knownProviders(), providerToken)
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

	urnVal := outputs.GetAttr("urn")
	if urnVal.Type() != cty.String {
		return fmt.Errorf("resource %q missing URN", resKey)
	}
	urn := resource.URN(urnVal.AsString())

	// Build __self__ resource reference
	var selfID property.Value
	if resSchema.IsComponent && !isProviderResource {
		selfID = property.New(property.Null)
	} else {
		idVal := outputs.GetAttr("id")
		if idVal.Type() == cty.String {
			selfID = property.New(idVal.AsString())
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

	userArgs, diags := transform.EvalFunctionWithSchema(call.Config, &filteredFunc,
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
	ctyOutputs, err := transform.FunctionOutputToCty(ret, method.Function, e.dryRun)
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
func (e *Engine) invokeFunction(ctx context.Context, req InvokeRequest) (property.Map, error) {
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

	return resp.Return, nil
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

func (a *moduleLoaderAdapter) LoadModule(source, workDir string) (*graph.LoadedModule, error) {
	loaded, err := a.loader.LoadModule(source, workDir)
	if err != nil {
		return nil, err
	}
	return &graph.LoadedModule{
		Config:     loaded.Config,
		SourcePath: loaded.SourcePath,
	}, nil
}

// forEachModuleInstance iterates over all instances of the module identified by node.ModuleInfo.Prefix.
func (e *Engine) forEachModuleInstance(node *graph.Node, fn func(inst *moduleInstance) error) error {
	instances, ok := e.moduleInstances.Get(node.ModuleInfo.Prefix)
	if !ok {
		return fmt.Errorf("no module instances for prefix %q", node.ModuleInfo.Prefix)
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
	varName := strings.TrimPrefix(node.Key, modInfo.Prefix+"var.")

	moduleInputAttrs, _ := modInfo.Module.Config.JustAttributes()
	inputAttr, hasInput := moduleInputAttrs[varName]

	return e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		var val cty.Value

		if hasInput {
			// Evaluate the input expression in the parent context.
			// The parent context is the eval context that contains count/each for this instance.
			parentCtx := e.evaluator.Context()
			if modInfo.ParentPrefix != "" {
				parentCtx = inst.EvalCtx
			}

			// Need the parent eval context, not the module instance's context.
			// For root-level modules, the parent is e.evaluator.Context() with count/each set.
			var diags hcl.Diagnostics
			parentHCL := parentCtx.HCLContext()
			if inst.Index != nil {
				parentHCL = e.evaluator.Context().Clone().HCLContext()
				// We need a parent context with the right count/each set
			}
			// Use the root evaluator's context for parent expressions
			val, diags = inputAttr.Expr.Value(e.evaluator.Context().HCLContext())
			_ = parentHCL // TODO: for nested modules, use parent instance context
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
			} else if v.Nullable {
				val = cty.NullVal(cty.DynamicPseudoType)
			} else {
				return fmt.Errorf("variable %q is required but no value was provided", varName)
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

	// If this is a nested module, look up the parent instance URN.
	if modInfo.ParentPrefix != "" {
		parentInstances, ok := e.moduleInstances.Get(modInfo.ParentPrefix)
		if ok && len(parentInstances) > 0 {
			parentURN = parentInstances[0].URN
			parentEvalCtx = parentInstances[0].EvalCtx
		}
	}

	baseKey := modInfo.ParentPrefix + "module." + modInfo.ModuleName

	// Evaluate module inputs for the component resource registration
	inputs := make(map[string]property.Value)
	attrs, _ := mod.Config.JustAttributes()
	for name, attr := range attrs {
		val, diags := attr.Expr.Value(parentEvalCtx.HCLContext())
		if diags.HasErrors() {
			continue
		}
		pv, err := transform.CtyToPropertyValue(val)
		if err == nil {
			inputs[name] = pv
		}
	}

	// No count/for_each: single instance.
	if mod.Count == nil && mod.ForEach == nil {
		componentOpts := &ResourceOptions{Parent: parentURN}
		componentURN, _, _, err := e.registerComponentResource(ctx, componentType, extractResourceName(baseKey), property.NewMap(inputs), componentOpts)
		if err != nil {
			return fmt.Errorf("registering module component: %w", err)
		}

		instCtx := eval.NewContext(
			modInfo.SourcePath, e.workDir,
			e.stackName, e.projectName, e.organization,
		)

		e.moduleInstances.Set(modInfo.Prefix, []*moduleInstance{{
			Key:     baseKey,
			EvalCtx: instCtx,
			URN:     componentURN,
		}})
		return nil
	}

	// Count expansion
	if mod.Count != nil {
		countVal, diags := mod.Count.Value(parentEvalCtx.HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module count: %s", diags.Error())
		}
		if !countVal.Type().Equals(cty.Number) {
			return fmt.Errorf("module count must be a number")
		}
		count, _ := countVal.AsBigFloat().Int64()
		var instances []*moduleInstance
		for i := int64(0); i < count; i++ {
			idx := int(i)
			instKey := fmt.Sprintf("%s[%d]", baseKey, i)
			componentOpts := &ResourceOptions{Parent: parentURN}
			componentURN, _, _, err := e.registerComponentResource(ctx, componentType, extractResourceName(instKey), property.NewMap(inputs), componentOpts)
			if err != nil {
				return fmt.Errorf("registering module component %s: %w", instKey, err)
			}
			instCtx := eval.NewContext(
				modInfo.SourcePath, e.workDir,
				e.stackName, e.projectName, e.organization,
			)
			instCtx.SetCount(idx)
			instances = append(instances, &moduleInstance{
				Key:     instKey,
				EvalCtx: instCtx,
				URN:     componentURN,
				Index:   &idx,
			})
		}
		e.moduleInstances.Set(modInfo.Prefix, instances)
		return nil
	}

	// ForEach expansion
	forEachVal, diags := mod.ForEach.Value(parentEvalCtx.HCLContext())
	if diags.HasErrors() {
		return fmt.Errorf("evaluating module for_each: %s", diags.Error())
	}
	if !forEachVal.CanIterateElements() {
		return fmt.Errorf("module for_each must be a set or map")
	}
	var instances []*moduleInstance
	it := forEachVal.ElementIterator()
	for it.Next() {
		k, v := it.Element()
		keyStr := k.AsString()
		instKey := fmt.Sprintf("%s[\"%s\"]", baseKey, keyStr)
		componentOpts := &ResourceOptions{Parent: parentURN}
		componentURN, _, _, err := e.registerComponentResource(ctx, componentType, extractResourceName(instKey), property.NewMap(inputs), componentOpts)
		if err != nil {
			return fmt.Errorf("registering module component %s: %w", instKey, err)
		}
		instCtx := eval.NewContext(
			modInfo.SourcePath, e.workDir,
			e.stackName, e.projectName, e.organization,
		)
		instCtx.SetEach(k, v)
		instances = append(instances, &moduleInstance{
			Key:     instKey,
			EvalCtx: instCtx,
			URN:     componentURN,
			EachKey: &k,
			EachVal: &v,
		})
	}
	e.moduleInstances.Set(modInfo.Prefix, instances)
	return nil
}

// processModuleOutput evaluates a module output in each instance and stores it in the parent context.
func (e *Engine) processModuleOutput(_ context.Context, node *graph.Node) error {
	output := node.Output
	modInfo := node.ModuleInfo
	outputName := strings.TrimPrefix(node.Key, modInfo.Prefix+"output.")
	mod := modInfo.Module
	isCounted := mod.Count != nil
	isForEach := mod.ForEach != nil

	err := e.forEachModuleInstance(node, func(inst *moduleInstance) error {
		val, diags := output.Value.Value(inst.EvalCtx.HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module output %s: %s", outputName, diags.Error())
		}
		if inst.Outputs == nil {
			inst.Outputs = make(map[string]cty.Value)
		}
		inst.Outputs[outputName] = val
		return nil
	})
	if err != nil {
		return err
	}

	// Eagerly publish outputs to the parent context so other module variables
	// can reference them before the completion node runs.
	parentCtx := e.evaluator.Context()
	instances, ok := e.moduleInstances.Get(modInfo.Prefix)
	if !ok {
		return nil
	}

	if !isCounted && !isForEach {
		if len(instances) == 1 {
			if v, has := instances[0].Outputs[outputName]; has {
				parentCtx.SetModuleOutput(modInfo.ModuleName, outputName, v)
			}
		}
	} else if isCounted {
		// Rebuild the full tuple from all collected outputs so far.
		tupleVals := make([]cty.Value, len(instances))
		for i, inst := range instances {
			if len(inst.Outputs) > 0 {
				tupleVals[i] = cty.ObjectVal(inst.Outputs)
			} else {
				tupleVals[i] = cty.EmptyObjectVal
			}
		}
		if len(tupleVals) > 0 {
			parentCtx.SetModule(modInfo.ModuleName, cty.TupleVal(tupleVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName, cty.EmptyTupleVal)
		}
	} else {
		// ForEach: rebuild the map.
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
			parentCtx.SetModule(modInfo.ModuleName, cty.ObjectVal(mapVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName, cty.EmptyObjectVal)
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

	instances, ok := e.moduleInstances.Get(modInfo.Prefix)
	if !ok {
		return fmt.Errorf("no module instances for prefix %q", modInfo.Prefix)
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

	// Assemble module value in parent eval context.
	parentCtx := e.evaluator.Context()

	if !isCounted && !isForEach {
		// Single instance: module.X is an object of outputs.
		if len(instances) == 1 {
			for k, v := range instances[0].Outputs {
				parentCtx.SetModuleOutput(modInfo.ModuleName, k, v)
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
			parentCtx.SetModule(modInfo.ModuleName, cty.TupleVal(tupleVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName, cty.EmptyTupleVal)
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
			parentCtx.SetModule(modInfo.ModuleName, cty.ObjectVal(mapVals))
		} else {
			parentCtx.SetModule(modInfo.ModuleName, cty.EmptyObjectVal)
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

// camelToSnake converts a camelCase string to snake_case.
// For example, "publicIp" becomes "public_ip", "vpcSecurityGroupIds" becomes "vpc_security_group_ids".
func camelToSnake(s string) string {
	var result strings.Builder
	for i, r := range s {
		if i > 0 && r >= 'A' && r <= 'Z' {
			result.WriteByte('_')
		}
		if r >= 'A' && r <= 'Z' {
			result.WriteRune(r + 32) // Convert to lowercase
		} else {
			result.WriteRune(r)
		}
	}
	return result.String()
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
	engine := NewEngine(config, opts)
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

// evaluateCheckRules evaluates a list of preconditions or postconditions.
// Returns an error if any check fails, with the evaluated error message.
func (e *Engine) evaluateCheckRules(
	rules []*ast.CheckRule,
	resourceName string,
	phase string,
) error {
	for i, rule := range rules {
		// Evaluate the condition
		condVal, diags := e.evaluator.EvaluateExpression(rule.Condition)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating %s %d for %s: %s", phase, i+1, resourceName, diags.Error())
		}

		// Check if condition is true
		if condVal.Type() != cty.Bool {
			return fmt.Errorf("%s %d for %s: condition must be a boolean", phase, i+1, resourceName)
		}

		if condVal.True() {
			// Condition passed, continue to next rule
			continue
		}

		// Condition failed - evaluate the error message
		msgVal, msgDiags := e.evaluator.EvaluateExpression(rule.ErrorMessage)
		if msgDiags.HasErrors() {
			return fmt.Errorf("%s %d for %s failed (could not evaluate error message: %s)",
				phase, i+1, resourceName, msgDiags.Error())
		}

		var errMsg string
		if msgVal.Type() == cty.String {
			errMsg = msgVal.AsString()
		} else {
			errMsg = fmt.Sprintf("%s check failed", phase)
		}

		return fmt.Errorf("%s for %s: %s", phase, resourceName, errMsg)
	}

	return nil
}

// checkPulumiVersion checks if the Pulumi CLI version satisfies the required version range.
// The version requirement is specified via the pulumi block's requiredVersionRange attribute.
func (e *Engine) checkPulumiVersion(ctx context.Context) error {
	// Check if the pulumi block exists and has a version requirement
	if e.config.Pulumi == nil || e.config.Pulumi.RequiredVersionRange == nil {
		// No version requirement specified
		return nil
	}

	// Evaluate the requiredVersionRange expression
	versionVal, diags := e.evaluator.EvaluateExpression(e.config.Pulumi.RequiredVersionRange)
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
