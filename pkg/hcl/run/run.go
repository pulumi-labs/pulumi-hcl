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
	"slices"
	"strings"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/graph"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/modules"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi-language-hcl/pkg/util"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"github.com/zclconf/go-cty/cty/json"
)

// ResourceMonitor is the interface for registering resources with Pulumi.
// This matches the resource monitor interface used by the Pulumi engine.
type ResourceMonitor interface {
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
}

// InvokeResponse contains the result of invoking a function.
type InvokeResponse struct {
	Return   property.Map
	Failures []string
}

// CallRequest contains the parameters for invoking a method on a resource.
type CallRequest struct {
	Token    string
	Args     property.Map
	Provider string
}

// CallResponse contains the result of invoking a method on a resource.
type CallResponse struct {
	Return   property.Map
	Failures []string
}

// inheritableOpts holds the resource options that child resources can inherit from their parent.
type inheritableOpts struct {
	Provider       string
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

	// moduleOutputs maps module keys to their output values.
	moduleOutputs map[string]cty.Value

	// parentURN is the parent resource URN (for child modules).
	parentURN string
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
		moduleLoader:            modules.NewLoader(),
		moduleOutputs:           make(map[string]cty.Value),
	}

	return engine
}

// Run executes the HCL program.
func (e *Engine) Run(ctx context.Context) error {
	// Register the root stack resource to get its URN for outputs
	if err := e.registerStack(ctx); err != nil {
		return fmt.Errorf("registering stack: %w", err)
	}

	// Build the dependency graph
	g, err := graph.BuildFromConfig(e.config)
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
	case graph.NodeTypeModule:
		return e.processModule(ctx, node)
	case graph.NodeTypeCall:
		return e.processCall(ctx, node)
	case graph.NodeTypeOutput:
		// Outputs are processed after the main loop
		return nil
	case graph.NodeTypeProvider:
		return e.processProvider(ctx, node)
	case graph.NodeTypeBuiltin:
		// We don't need to evaluate builtins
		return nil
	case graph.NodeTypeUnknown:
		return errors.New("unknown node type")
	default:
		return fmt.Errorf("unknown node type: %v", node.Type)
	}
}

func (e *Engine) processGraph(ctx context.Context, g *graph.Graph) error {
	if err := g.InjectAfter(e.checkPulumiVersion, graph.NodeTypeVariable); err != nil {
		return err
	}
	return g.Walk(ctx, e.processNode)
}

// processVariable processes a variable definition.
func (e *Engine) processVariable(_ context.Context, node *graph.Node) error {
	v := node.Variable
	if v == nil {
		return fmt.Errorf("variable node missing Variable field")
	}

	varName := node.Key[4:] // Remove "var." prefix
	var val cty.Value
	var isSecret bool
	var valueSource string

	// Variable value precedence (highest to lowest):
	// 0. Already there (as produced by the parent when this is a child module)
	// 1. Environment variable TF_VAR_<name>
	// 2. Pulumi stack config (projectName:<name>)
	// 3. Default value

	if e.evaluator.Context().HCLContext().Variables["var"].Type().HasAttribute(varName) {
		// This would imply that there are multiple variables setting the same name.
		if e.parentURN == "" {
			return fmt.Errorf("%q already evaluated", varName)
		}

		// The variable is already set, so we don't need to do anything.
		return nil
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

	// Evaluate the local value expression, intercepting can() calls.
	val, diags := e.evaluator.EvaluateExpression(local.Value)
	if diags.HasErrors() {
		return fmt.Errorf("evaluating local value: %s", diags.Error())
	}

	// Store in eval context
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

	// Construct the provider type token: "pulumi:providers:<provider-name>"
	typeToken := "pulumi:providers:" + provider.Name

	// Evaluate provider configuration
	attrs, _ := provider.Config.JustAttributes()
	inputs := make(map[string]property.Value)

	// TODO: This needs to lookup a resource schema & then use transform methods

	for name, attr := range attrs {
		// Skip the alias attribute as it's not part of the provider configuration
		if name == "alias" {
			continue
		}

		val, diags := e.evaluator.EvaluateExpression(attr.Expr)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating provider attribute %s: %s", name, diags.Error())
		}

		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting provider attribute %s: %w", name, err)
		}

		inputs[name] = pv
	}

	// Register the provider resource
	// The logical name for the provider is its alias (stored in node.Key)
	logicalName := provider.Alias
	if logicalName == "" {
		logicalName = provider.Name
	}

	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:   typeToken,
		Name:   logicalName,
		Inputs: property.NewMap(inputs),
		Custom: true,
	})
	if err != nil {
		return fmt.Errorf("registering provider %s: %w", node.Key, err)
	}

	// Provider resources should return an ID from the engine
	providerID := resp.ID

	// Store the provider outputs in the same format as regular resources
	outputObj := make(map[string]cty.Value)
	outputObj["id"] = cty.StringVal(providerID)
	outputObj["urn"] = cty.StringVal(resp.URN)

	for k, v := range resp.Outputs.All {
		snakeKey := camelToSnake(string(k))
		outputObj[snakeKey] = transform.PropertyValueToCty(v)
	}

	e.resourceOutputs.Set(node.Key, cty.ObjectVal(outputObj))
	e.evaluator.Context().SetResource(node.Key, cty.ObjectVal(outputObj))

	return nil
}

// processResource processes a resource definition.
func (e *Engine) processResource(ctx context.Context, node *graph.Node) error {
	res := node.Resource
	if res == nil {
		return fmt.Errorf("resource node missing Resource field")
	}

	// Resolve the resource type to a Pulumi type token.
	resSchema, err := packages.ResolveResource(ctx, e.pkgLoader, res.Type)
	if err != nil {
		return fmt.Errorf("resolving resource type %s: %w", res.Type, err)
	}

	// Check for count/for_each expansion
	expander := graph.NewResourceExpander()

	if res.Count != nil {
		count, diags := e.evaluator.EvaluateCount(res.Count)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating count: %s", diags.Error())
		}
		expander.SetCount(node.Key, count)
	}

	if res.ForEach != nil {
		forEach, diags := e.evaluator.EvaluateForEach(res.ForEach)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating for_each: %s", diags.Error())
		}
		expander.SetForEach(node.Key, forEach)
	}

	// Expand the resource
	result := expander.Expand(node)

	// Register each instance
	for _, instance := range result.Instances {
		if err := e.registerResourceInstance(ctx, res, resSchema, instance); err != nil {
			return fmt.Errorf("registering %s: %w", instance.Key, err)
		}
	}

	return nil
}

// registerResourceInstance registers a single resource instance with Pulumi.
func (e *Engine) registerResourceInstance(
	ctx context.Context,
	res *ast.Resource,
	resSchema *schema.Resource,
	instance *graph.ExpandedResource,
) error {
	// Set up instance-specific context (count.index, each.key, etc.)
	if instance.Index != nil {
		e.evaluator.Context().SetCount(*instance.Index)
		defer e.evaluator.Context().ClearCount()
	}
	if instance.EachKey != nil && instance.EachValue != nil {
		e.evaluator.Context().SetEach(*instance.EachKey, *instance.EachValue)
		defer e.evaluator.Context().ClearEach()
	}

	dependsOn := make(map[string][]string)
	addToDependsOn := func(prop, urn string) {
		idx, found := slices.BinarySearch(dependsOn[prop], urn)
		if found {
			return
		}
		dependsOn[prop] = slices.Insert(dependsOn[prop], idx, urn)
	}

	resourceInputs, diags := transform.EvalResourceWithSchema(res.Config, resSchema,
		func(propKey resource.PropertyKey, expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
			val, diags := e.evaluator.EvaluateExpression(expr)
			if diags.HasErrors() {
				return val, diags
			}

			// Extract dependencies from this attribute's expression
			for _, dep := range eval.ExtractDependencies(expr) {
				// Look up the URN for this dependency
				if resOutputs, ok := e.resourceOutputs.Get(dep); ok {
					if urnVal := resOutputs.GetAttr("urn"); urnVal.Type() == cty.String {
						addToDependsOn(string(propKey), urnVal.AsString())
					}
				}
				// For data source dependencies, inherit their dependencies transitively
				if dsKey, ok := strings.CutPrefix(dep, "data."); ok {
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

	// For provider resources, plugin_download_url is a configuration input property but
	// the parser extracts it as a meta-argument. Re-add it to the resource inputs.
	if strings.HasPrefix(res.Type, "pulumi_providers_") && res.PluginDownloadURL != nil {
		val, valDiags := res.PluginDownloadURL.Value(e.evaluator.Context().HCLContext())
		if !valDiags.HasErrors() && val.Type() == cty.String {
			resourceInputs = resourceInputs.Set("pluginDownloadURL", property.New(val.AsString()))
		}
	}

	// Build resource options
	opts := e.buildResourceOptions(res, instance)
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

	// Set version from required_providers if not explicitly set by resource options.
	if opts.Version == "" {
		pkgName := packageNameFromResourceType(res.Type)
		if e.config.Terraform != nil {
			if req, ok := e.config.Terraform.RequiredProviders[pkgName]; ok && req.Version != "" {
				opts.Version = ExtractSemverFromConstraint(req.Version)
			}
		}
	}

	// Set plugin download URL from package schema if not explicitly set.
	if opts.PluginDownloadURL == "" && resSchema.PackageReference != nil {
		opts.PluginDownloadURL = resSchema.PackageReference.PluginDownloadURL()
	}

	// Evaluate preconditions before resource creation
	if len(res.Preconditions) > 0 {
		if err := e.evaluateCheckRules(res.Preconditions, instance.Key, "precondition"); err != nil {
			return err
		}
	}

	// Register the resource
	// Extract the resource name from the instance key (e.g., "pulumi_stash.myStash" -> "myStash")
	resourceName := extractResourceName(instance.Key)

	urn, id, outputs, err := e.registerResource(ctx, resSchema.Token, resourceName, resourceInputs, opts)
	if err != nil {
		return fmt.Errorf("registering resource: %w", err)
	}

	outputs = outputs.Delete("id", "urn")

	outputs, err = e.resolveResourceRefsInOutputs(ctx, outputs, resSchema)
	if err != nil {
		return fmt.Errorf("resolving resource references in outputs: %w", err)
	}

	// Store outputs for future references
	outputObj, err := transform.ResourceOutputToCty(outputs, resSchema, e.dryRun)
	if err != nil {
		return fmt.Errorf("converting resource outputs to HCL types: %w", err)
	}
	outputObj["id"] = cty.StringVal(id)
	outputObj["urn"] = cty.StringVal(urn)

	e.resourceOutputs.Set(instance.Key, cty.ObjectVal(outputObj))

	// Store inheritable options so child resources can inherit from this resource.
	iOpts := inheritableOpts{Provider: opts.Provider}
	if opts.Protect {
		iOpts.Protect = new(true)
	}
	iOpts.RetainOnDelete = opts.RetainOnDelete
	e.resourceInheritableOpts.Set(instance.Key, iOpts)

	// Also store in eval context for expression references
	e.evaluator.Context().SetResource(instance.Key, cty.ObjectVal(outputObj))

	// Evaluate postconditions after resource creation
	// Set self to the resource outputs so postconditions can reference self
	if len(res.Postconditions) > 0 {
		e.evaluator.Context().SetSelf(cty.ObjectVal(outputObj))
		defer e.evaluator.Context().ClearSelf()
		if err := e.evaluateCheckRules(res.Postconditions, instance.Key, "postcondition"); err != nil {
			return err
		}
	}

	// Process provisioners after resource creation
	if len(res.Provisioners) > 0 {
		if err := e.processProvisioners(ctx, res, urn, cty.ObjectVal(outputObj), instance.Key); err != nil {
			return fmt.Errorf("processing provisioners: %w", err)
		}
	}

	return nil
}

// buildResourceOptions builds resource options from the resource definition.
func (e *Engine) buildResourceOptions(res *ast.Resource, instance *graph.ExpandedResource) *ResourceOptions {
	opts := &ResourceOptions{}

	// Default parent: use the module component URN for child engines, or the stack URN for top-level.
	if e.parentURN != "" {
		opts.Parent = e.parentURN
	} else {
		opts.Parent = e.stackURN
	}

	// Handle depends_on - resolve to URNs
	for _, dep := range res.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(depKey); ok {
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
	}

	// Handle parent resource reference
	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(depKey); ok {
				urnVal := outputs.GetAttr("urn")
				if urnVal.Type() == cty.String {
					opts.Parent = urnVal.AsString()
				}
			}
		}
	}

	// Handle provider reference
	if res.Provider != nil {
		providerKey := res.Provider.Name
		if res.Provider.Alias != "" {
			providerKey = res.Provider.Name + "." + res.Provider.Alias
		}
		// Look up the provider URN and ID
		if providerOutputs, ok := e.resourceOutputs.Get(providerKey); ok {
			// Provider reference format: "<urn>::<id>"
			urnVal := providerOutputs.GetAttr("urn")
			idVal := providerOutputs.GetAttr("id")
			if urnVal.Type() == cty.String && idVal.Type() == cty.String {
				opts.Provider = urnVal.AsString() + "::" + idVal.AsString()
			}
		}
	}

	// Handle providers list (for component resources)
	for _, traversal := range res.Providers {
		providerKey := graph.FormatTraversal(traversal)
		if providerKey == "" {
			continue
		}
		if providerOutputs, ok := e.resourceOutputs.Get(providerKey); ok {
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

	// Handle additional_secret_outputs
	if res.AdditionalSecretOutputs != nil {
		secretOutputsVal, diags := res.AdditionalSecretOutputs.Value(e.evaluator.Context().HCLContext())
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

	// Handle retain_on_delete
	if res.RetainOnDelete != nil {
		val, diags := res.RetainOnDelete.Value(e.evaluator.Context().HCLContext())
		if !diags.HasErrors() && val.Type() == cty.Bool {
			b := val.True()
			opts.RetainOnDelete = &b
		}
	}

	// Handle deleted_with - resolve to URN
	if res.DeletedWith != nil {
		depKey := graph.FormatTraversal(res.DeletedWith)
		if depKey != "" {
			if outputs, ok := e.resourceOutputs.Get(depKey); ok {
				urnVal := outputs.GetAttr("urn")
				if urnVal.Type() == cty.String {
					opts.DeletedWith = urnVal.AsString()
				}
			}
		}
	}

	// Handle replace_with - resolve each resource reference to URN
	for _, ref := range res.ReplaceWith {
		depKey := graph.FormatTraversal(ref)
		if depKey == "" {
			continue
		}
		if outputs, ok := e.resourceOutputs.Get(depKey); ok {
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

	// Handle replacement_trigger
	if res.ReplacementTrigger != nil {
		val, diags := res.ReplacementTrigger.Value(e.evaluator.Context().HCLContext())
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

	// Handle env_var_mappings
	if res.EnvVarMappings != nil {
		val, diags := res.EnvVarMappings.Value(e.evaluator.Context().HCLContext())
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

	// Handle version
	if res.Version != nil {
		val, diags := res.Version.Value(e.evaluator.Context().HCLContext())
		if !diags.HasErrors() && val.Type() == cty.String {
			opts.Version = val.AsString()
		}
	}

	// Handle plugin_download_url as a resource option.
	// For provider resources (pulumi_providers_*), plugin_download_url is a provider
	// configuration property, not a resource option, so it should not be set here.
	if res.PluginDownloadURL != nil && !strings.HasPrefix(res.Type, "pulumi_providers_") {
		val, diags := res.PluginDownloadURL.Value(e.evaluator.Context().HCLContext())
		if !diags.HasErrors() && val.Type() == cty.String {
			opts.PluginDownloadURL = val.AsString()
		}
	}

	// Inherit options from parent resource if not explicitly set.
	if res.ResourceParent != nil {
		depKey := graph.FormatTraversal(res.ResourceParent)
		if depKey != "" {
			if parentOpts, ok := e.resourceInheritableOpts.Get(depKey); ok {
				if res.Provider == nil && opts.Provider == "" && parentOpts.Provider != "" {
					opts.Provider = parentOpts.Provider
				}
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

	return opts
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

func extractResourceName(key string) string {
	// Find the first dot to split type from name
	dotIndex := strings.Index(key, ".")
	if dotIndex == -1 {
		// No dot found, return the whole key
		return key
	}
	// Return everything after the dot
	return key[dotIndex+1:]
}

// formatTraversalForIgnoreChanges formats a traversal for ignore_changes.
// Handles both relative traversals (just "tags") and absolute ones.
func formatTraversalForIgnoreChanges(traversal hcl.Traversal) string {
	if len(traversal) == 0 {
		return ""
	}

	var parts []string
	for _, step := range traversal {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			parts = append(parts, s.Name)
		case hcl.TraverseAttr:
			parts = append(parts, s.Name)
		case hcl.TraverseIndex:
			// For index traversals, add [key] or [index]
			key := s.Key
			if key.Type() == cty.String {
				parts = append(parts, fmt.Sprintf("[%q]", key.AsString()))
			} else if key.Type() == cty.Number {
				bf := key.AsBigFloat()
				if i64, acc := bf.Int64(); acc == 0 {
					parts = append(parts, fmt.Sprintf("[%d]", i64))
				}
			}
		}
	}

	return strings.Join(parts, ".")
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
	funcSchema, err := packages.ResolveFunction(ctx, e.pkgLoader, ds.Type)
	if err != nil {
		return fmt.Errorf("resolving data source type %s: %w", ds.Type, err)
	}

	var allDeps []resource.URN

	inputs, diags := transform.EvalFunctionWithSchema(ds.Config, funcSchema,
		func(propKey resource.PropertyKey, expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
			val, diags := e.evaluator.EvaluateExpression(expr)
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

	invokeReq := InvokeRequest{Token: funcSchema.Token, Args: inputs}

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
	var resSchema *schema.Resource
	var isProvider bool
	var isProviderResource bool // true for resource "pulumi_providers_*" blocks

	for k, res := range e.config.Resources {
		if res.Name == call.ResourceName {
			resKey = k
			var err error
			resSchema, err = packages.ResolveResource(ctx, e.pkgLoader, res.Type)
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
			isProvider = true
			var err error
			// Providers use their name as the key
			providerToken := "pulumi_providers_" + e.config.Providers[call.ResourceName].Name
			resSchema, err = packages.ResolveResource(ctx, e.pkgLoader, providerToken)
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

	// Determine the provider reference for routing the call to the right provider instance.
	// For provider resources, the provider IS the resource itself (urn::id).
	// For custom resources, the provider is inherited from the resource's own provider.
	var callProvider string
	if isProvider || isProviderResource {
		idVal := outputs.GetAttr("id")
		if urnVal.Type() == cty.String && idVal.Type() == cty.String {
			callProvider = urnVal.AsString() + "::" + idVal.AsString()
		}
	} else {
		if iOpts, ok := e.resourceInheritableOpts.Get(resKey); ok {
			callProvider = iOpts.Provider
		}
	}

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
		func(_ resource.PropertyKey, expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
			return e.evaluator.EvaluateExpression(expr)
		})
	if diags.HasErrors() {
		return fmt.Errorf("evaluating call arguments for %s.%s: %s", call.ResourceName, call.MethodName, diags.Error())
	}

	ret, err := e.callMethod(ctx, CallRequest{
		Token:    method.Function.Token,
		Args:     userArgs.Set("__self__", selfRef),
		Provider: callProvider,
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
func (e *Engine) processModule(ctx context.Context, node *graph.Node) error {
	mod := node.Module
	if mod == nil {
		return fmt.Errorf("module node missing Module field")
	}

	// Expand the module for count/for_each
	instances, err := e.expandModuleInstances(mod)
	if err != nil {
		return fmt.Errorf("expanding module instances: %w", err)
	}

	for _, instance := range instances {
		if err := e.processModuleInstance(ctx, mod, instance); err != nil {
			return err
		}
	}

	return nil
}

// expandedModule represents an expanded module instance (for count/for_each).
type expandedModule struct {
	Key     string     // e.g., "module.vpc" or "module.vpc[0]"
	Index   *int       // count index if using count
	EachKey *cty.Value // for_each key if using for_each
	EachVal *cty.Value // for_each value if using for_each
}

// expandModuleInstances expands a module for count/for_each.
func (e *Engine) expandModuleInstances(mod *ast.Module) ([]*expandedModule, error) {
	baseKey := "module." + mod.Name

	// Handle count
	if mod.Count != nil {
		countVal, diags := e.evaluator.EvaluateExpression(mod.Count)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating count: %s", diags.Error())
		}

		if !countVal.Type().Equals(cty.Number) {
			return nil, fmt.Errorf("count must be a number")
		}

		count, _ := countVal.AsBigFloat().Int64()
		if count < 0 {
			return nil, fmt.Errorf("count cannot be negative")
		}

		var instances []*expandedModule
		for i := int64(0); i < count; i++ {
			idx := int(i)
			instances = append(instances, &expandedModule{
				Key:   fmt.Sprintf("%s[%d]", baseKey, i),
				Index: &idx,
			})
		}
		return instances, nil
	}

	// Handle for_each
	if mod.ForEach != nil {
		forEachVal, diags := e.evaluator.EvaluateExpression(mod.ForEach)
		if diags.HasErrors() {
			return nil, fmt.Errorf("evaluating for_each: %s", diags.Error())
		}

		if !forEachVal.CanIterateElements() {
			return nil, fmt.Errorf("for_each must be a set or map")
		}

		var instances []*expandedModule
		it := forEachVal.ElementIterator()
		for it.Next() {
			k, v := it.Element()
			keyStr := k.AsString()
			instances = append(instances, &expandedModule{
				Key:     fmt.Sprintf("%s[\"%s\"]", baseKey, keyStr),
				EachKey: &k,
				EachVal: &v,
			})
		}
		return instances, nil
	}

	// No count or for_each - single instance
	return []*expandedModule{{Key: baseKey}}, nil
}

// processModuleInstance processes a single module instance.
func (e *Engine) processModuleInstance(ctx context.Context, mod *ast.Module, instance *expandedModule) error {
	// Set up instance-specific context (count.index, each.key, etc.)
	if instance.Index != nil {
		e.evaluator.Context().SetCount(*instance.Index)
		defer e.evaluator.Context().ClearCount()
	}
	if instance.EachKey != nil && instance.EachVal != nil {
		e.evaluator.Context().SetEach(*instance.EachKey, *instance.EachVal)
		defer e.evaluator.Context().ClearEach()
	}

	// Load the module source
	loadedModule, err := e.moduleLoader.LoadModule(mod.Source, e.workDir)
	if err != nil {
		return fmt.Errorf("loading module %s: %w", mod.Name, err)
	}

	// Evaluate module inputs from the module block
	inputs := make(map[string]property.Value)
	attrs, _ := mod.Config.JustAttributes()
	for name, attr := range attrs {
		val, diags := e.evaluator.EvaluateExpression(attr.Expr)
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module input %s: %s", name, diags.Error())
		}
		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting module input %s: %w", name, err)
		}
		inputs[name] = pv
	}

	// Register the module as a component resource with a dynamic type token.
	// Format: {projectName}:modules:{moduleName}
	// This enables proper identification in the Pulumi state and UI.
	moduleName := mod.Name
	if idx := strings.LastIndex(moduleName, "/"); idx != -1 {
		moduleName = moduleName[idx+1:]
	}
	componentType := fmt.Sprintf("%s:modules:%s", e.projectName, moduleName)
	componentOpts := &ResourceOptions{
		Parent: e.parentURN,
	}

	// Handle depends_on
	for _, dep := range mod.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey != "" {
			componentOpts.DependsOn = append(componentOpts.DependsOn, depKey)
		}
	}

	componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instance.Key, property.NewMap(inputs), componentOpts)
	if err != nil {
		return fmt.Errorf("registering module component: %w", err)
	}

	// Create a child engine to execute the module
	childEngine := e.createChildEngine(loadedModule.Config, componentURN, loadedModule.SourcePath)

	// Set up the child engine's variables with module inputs
	if diags := childEngine.setModuleInputs(attrs, e.evaluator.Context()); diags.HasErrors() {
		return diags
	}

	// Execute the module
	if err := childEngine.runModule(ctx); err != nil {
		return fmt.Errorf("executing module %s: %w", mod.Name, err)
	}

	// Collect module outputs and make them available
	moduleOutputs := childEngine.collectModuleOutputs()
	e.moduleOutputs[instance.Key] = moduleOutputs

	// Set module in eval context using just the module name or indexed key
	// instance.Key is like "module.vpc" or "module.vpc[0]"
	// We need to store at "vpc" or "vpc[0]" for module.vpc.output_name to work
	moduleRefKey := strings.TrimPrefix(instance.Key, "module.")
	e.evaluator.Context().SetModule(moduleRefKey, moduleOutputs)

	// Register the component outputs
	if e.resmon != nil {
		outputProps := make(map[string]property.Value)
		if moduleOutputs.Type().IsObjectType() {
			for name, val := range moduleOutputs.AsValueMap() {
				pv, err := transform.CtyToPropertyValue(val)
				if err == nil {
					outputProps[name] = pv
				}
			}
		}
		if err := e.resmon.RegisterResourceOutputs(ctx, componentURN, property.NewMap(outputProps)); err != nil {
			return fmt.Errorf("registering module outputs: %w", err)
		}
	}

	return nil
}

// registerComponentResource registers a component (non-custom) resource.
func (e *Engine) registerComponentResource(
	ctx context.Context,
	typeToken string,
	name string,
	inputs property.Map,
	opts *ResourceOptions,
) (string, string, property.Map, error) {
	if e.resmon == nil { // TODO: Remove this check
		// No resource monitor - return synthetic values for testing
		urn := fmt.Sprintf("urn:pulumi:%s::%s::%s::%s",
			e.stackName, e.projectName, typeToken, name)
		return urn, "", inputs, nil
	}

	// Build dependencies list
	deps := opts.DependsOn

	// Register with the resource monitor - Custom=false for components
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:         typeToken,
		Name:         name,
		Inputs:       inputs,
		Dependencies: deps,
		Parent:       opts.Parent,
		// Note: Custom=false for components, but we handle this in server.go
	})
	if err != nil {
		return "", "", property.Map{}, err
	}

	return resp.URN, resp.ID, resp.Outputs, nil
}

// createChildEngine creates a child engine for executing a module.
func (e *Engine) createChildEngine(config *ast.Config, parentURN string, moduleDir string) *Engine {
	return &Engine{
		config: config,
		evaluator: eval.NewEvaluator(eval.NewContext(moduleDir, moduleDir,
			e.stackName, e.projectName, e.organization)),
		pkgLoader:               e.pkgLoader,
		resmon:                  e.resmon,
		resourceOutputs:         util.NewSyncMap[string, cty.Value](),
		resourceInheritableOpts: util.NewSyncMap[string, inheritableOpts](),
		stackOutputs:            make(map[string]property.Value),
		projectName:             e.projectName,
		stackName:               e.stackName,
		organization:            e.organization,
		dryRun:                  e.dryRun,
		workDir:                 moduleDir,
		pulumiConfig:            e.pulumiConfig,
		configSecretKeys:        e.configSecretKeys,
		moduleLoader:            e.moduleLoader,
		moduleOutputs:           make(map[string]cty.Value),
		parentURN:               parentURN,
	}
}

// setModuleInputs sets up the module's variables with input values.
func (e *Engine) setModuleInputs(inputs hcl.Attributes, parentContext *eval.Context) hcl.Diagnostics {
	var diags hcl.Diagnostics
	for name, attr := range inputs {
		val, diag := attr.Expr.Value(parentContext.HCLContext())
		if !diag.HasErrors() {
			e.evaluator.Context().SetVariable(name, val)
		}
		diags = diags.Extend(diag)
	}
	return diags
}

// runModule executes a module's contents (without registering a stack).
func (e *Engine) runModule(ctx context.Context) error {
	// Build dependency graph for the module
	g, err := graph.BuildFromConfig(e.config)
	if err != nil {
		return fmt.Errorf("building module graph: %w", err)
	}

	return e.processGraph(ctx, g)
}

// collectModuleOutputs collects the module's output values.
func (e *Engine) collectModuleOutputs() cty.Value {
	outputMap := make(map[string]cty.Value)

	for name, output := range e.config.Outputs {
		val, diags := e.evaluator.EvaluateExpression(output.Value)
		if !diags.HasErrors() {
			outputMap[name] = val
		}
	}

	if len(outputMap) == 0 {
		return cty.EmptyObjectVal
	}

	return cty.ObjectVal(outputMap)
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

	// Build and validate the dependency graph
	g, err := graph.BuildFromConfig(config)
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
