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
	"fmt"
	"os"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/graph"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/modules"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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

	// RegisterResourceOutputs registers outputs on a resource (used for stack outputs).
	RegisterResourceOutputs(ctx context.Context, urn string, outputs resource.PropertyMap) error
}

// CustomTimeouts contains custom timeout values for resource operations.
type CustomTimeouts struct {
	Create float64 // Timeout in seconds for create operations
	Update float64 // Timeout in seconds for update operations
	Delete float64 // Timeout in seconds for delete operations
}

// RegisterResourceRequest contains the parameters for registering a resource.
type RegisterResourceRequest struct {
	Type                   string
	Name                   string
	Inputs                 resource.PropertyMap
	Dependencies           []string
	Protect                bool
	IgnoreChanges          []string
	Aliases                []string
	Provider               string
	Parent                 string
	DeleteBeforeReplace    bool
	DeleteBeforeReplaceDef bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts         *CustomTimeouts
	ImportId               string // Resource ID to import
}

// RegisterResourceResponse contains the result of registering a resource.
type RegisterResourceResponse struct {
	URN     string
	ID      string
	Outputs resource.PropertyMap
}

// InvokeRequest contains the parameters for invoking a function.
type InvokeRequest struct {
	Token string
	Args  resource.PropertyMap
}

// InvokeResponse contains the result of invoking a function.
type InvokeResponse struct {
	Return   resource.PropertyMap
	Failures []string
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
	resourceOutputs map[string]cty.Value

	// dataSourceOutputs maps data source keys to their output values.
	dataSourceOutputs map[string]cty.Value

	// stackOutputs collects outputs to be registered on the stack.
	stackOutputs resource.PropertyMap

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

	return &Engine{
		config:            config,
		evaluator:         eval.NewEvaluator(evalCtx),
		pkgLoader:         opts.SchemaLoader,
		resmon:            opts.ResourceMonitor,
		resourceOutputs:   make(map[string]cty.Value),
		dataSourceOutputs: make(map[string]cty.Value),
		stackOutputs:      make(resource.PropertyMap),
		projectName:       opts.ProjectName,
		stackName:         opts.StackName,
		organization:      opts.Organization,
		dryRun:            opts.DryRun,
		workDir:           opts.WorkDir,
		pulumiConfig:      opts.Config,
		configSecretKeys:  opts.ConfigSecretKeys,
		moduleLoader:      modules.NewLoader(),
		moduleOutputs:     make(map[string]cty.Value),
	}
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
		// For now, just log warnings for missing dependencies
		// They might be external (e.g., module outputs)
		for _, err := range errs {
			fmt.Printf("Warning: %v\n", err)
		}
	}

	// Topologically sort the nodes (we still need the sorted list for ordering)
	nodes, err := g.TopologicalSort()
	if err != nil {
		return fmt.Errorf("topological sort: %w", err)
	}

	// Process nodes in parallel where possible
	if err := e.processNodesParallel(ctx, nodes); err != nil {
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
		Inputs: resource.PropertyMap{},
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

	return e.resmon.RegisterResourceOutputs(ctx, e.stackURN, e.stackOutputs)
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
	case graph.NodeTypeOutput:
		// Outputs are processed after the main loop
		return nil
	case graph.NodeTypeProvider:
		// Provider configurations are handled during resource registration
		return nil
	default:
		return fmt.Errorf("unknown node type: %v", node.Type)
	}
}

// processNodesParallel processes nodes in parallel where possible.
// Variables and locals are processed first (sequentially, as they set up the eval context),
// then resources and data sources are processed in parallel respecting dependencies.
func (e *Engine) processNodesParallel(ctx context.Context, nodes []*graph.Node) error {

	// Separate nodes by type for phased processing
	var variables, locals, others []*graph.Node
	for _, node := range nodes {
		switch node.Type {
		case graph.NodeTypeVariable:
			variables = append(variables, node)
		case graph.NodeTypeLocal:
			locals = append(locals, node)
		case graph.NodeTypeOutput, graph.NodeTypeProvider:
			// Skip - handled elsewhere
		default:
			others = append(others, node)
		}
	}

	// Phase 1: Process variables sequentially (they're fast and set up context)
	for _, node := range variables {
		if err := e.processNode(ctx, node); err != nil {
			return fmt.Errorf("processing %s: %w", node.Key, err)
		}
	}

	// Phase 2: Process locals sequentially (they may depend on variables)
	// TODO: Could parallelize locals that don't depend on each other
	for _, node := range locals {
		if err := e.processNode(ctx, node); err != nil {
			return fmt.Errorf("processing %s: %w", node.Key, err)
		}
	}

	// Phase 3: Process resources and data sources in parallel
	if len(others) > 0 {
		if err := e.processNodesInParallel(ctx, others); err != nil {
			return err
		}
	}

	return nil
}

// processNodesInParallel processes a set of nodes in parallel, respecting dependencies.
func (e *Engine) processNodesInParallel(ctx context.Context, nodes []*graph.Node) error {
	// Build a map of node keys for quick lookup
	nodeSet := make(map[string]*graph.Node)
	for _, node := range nodes {
		nodeSet[node.Key] = node
	}

	// Track completion status
	var mu sync.Mutex
	completed := make(map[string]bool)
	var firstErr error

	// Create a channel to signal when nodes complete
	done := make(chan string, len(nodes))

	// Count pending dependencies for each node (only counting deps in our set)
	pendingDeps := make(map[string]int)
	dependents := make(map[string][]string) // node -> nodes that depend on it

	for _, node := range nodes {
		count := 0
		for _, dep := range node.Dependencies {
			// Only count dependencies that are in our processing set
			// (variables and locals are already processed)
			if _, inSet := nodeSet[dep]; inSet {
				count++
				dependents[dep] = append(dependents[dep], node.Key)
			}
		}
		pendingDeps[node.Key] = count
	}

	// Send initial signals for nodes with no dependencies (before starting goroutines)
	for _, node := range nodes {
		if pendingDeps[node.Key] == 0 {
			select {
			case done <- "":
			default:
			}
		}
	}

	// Start a goroutine for each node
	var wg sync.WaitGroup
	for _, node := range nodes {
		wg.Add(1)
		go func(n *graph.Node) {
			defer wg.Done()

			// Wait until all dependencies are satisfied
			for {
				mu.Lock()
				if firstErr != nil {
					mu.Unlock()
					return // Abort if there's been an error
				}
				pending := pendingDeps[n.Key]
				mu.Unlock()

				if pending == 0 {
					break
				}

				// Wait for a completion signal
				select {
				case <-ctx.Done():
					return
				case <-done:
					// A node completed, check again
				}
			}

			// Process this node
			err := e.processNode(ctx, n)

			mu.Lock()
			if err != nil && firstErr == nil {
				firstErr = fmt.Errorf("processing %s: %w", n.Key, err)
			}
			completed[n.Key] = true

			// Decrement pending count for dependents
			for _, depKey := range dependents[n.Key] {
				pendingDeps[depKey]--
			}
			mu.Unlock()

			// Signal completion (non-blocking)
			select {
			case done <- n.Key:
			default:
			}
		}(node)
	}

	wg.Wait()
	close(done)

	return firstErr
}

// processVariable processes a variable definition.
func (e *Engine) processVariable(ctx context.Context, node *graph.Node) error {
	v := node.Variable
	if v == nil {
		return fmt.Errorf("variable node missing Variable field")
	}

	varName := node.Key[4:] // Remove "var." prefix
	var val cty.Value
	var isSecret bool
	var valueSource string

	// Variable value precedence (highest to lowest):
	// 1. Environment variable TF_VAR_<name>
	// 2. Pulumi stack config (projectName:<name>)
	// 3. Default value

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
			for _, secretKey := range e.configSecretKeys {
				if secretKey == varName {
					isSecret = true
					break
				}
			}
		}
	}

	// If no value from env or config, use default
	if valueSource == "" {
		if v.Default != nil {
			var diags hcl.Diagnostics
			val, diags = v.Default.Value(e.evaluator.Context().HCLContext())
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
		condVal, diags := validation.Condition.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating validation condition %d for variable %q: %s", i+1, varName, diags.Error())
		}

		if condVal.Type() != cty.Bool {
			return fmt.Errorf("validation condition %d for variable %q must be boolean, got %s",
				i+1, varName, condVal.Type().FriendlyName())
		}

		if condVal.False() {
			// Get error message
			errMsgVal, diags := validation.ErrorMessage.Value(e.evaluator.Context().HCLContext())
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

// builtinResourceTypes maps HCL resource type names to Pulumi built-in type tokens.
// These resources are handled by the Pulumi engine itself and don't need schema resolution.
var builtinResourceTypes = map[string]string{
	"pulumi_stackreference": "pulumi:pulumi:StackReference",
}

// processResource processes a resource definition.
func (e *Engine) processResource(ctx context.Context, node *graph.Node) error {
	res := node.Resource
	if res == nil {
		return fmt.Errorf("resource node missing Resource field")
	}

	// Resolve the resource type to a Pulumi type token.
	// Check for built-in types first, then fall back to schema resolution.
	var typeToken string
	if token, ok := builtinResourceTypes[res.Type]; ok {
		typeToken = token
	} else {
		resType, err := packages.ResolveResource(ctx, e.pkgLoader, res.Type)
		if err != nil {
			return fmt.Errorf("resolving resource type %s: %w", res.Type, err)
		}
		typeToken = resType.Token
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
		if err := e.registerResourceInstance(ctx, res, typeToken, instance); err != nil {
			return fmt.Errorf("registering %s: %w", instance.Key, err)
		}
	}

	return nil
}

// registerResourceInstance registers a single resource instance with Pulumi.
func (e *Engine) registerResourceInstance(
	ctx context.Context,
	res *ast.Resource,
	typeToken string,
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

	// Evaluate resource configuration
	attrs, _ := res.Config.JustAttributes()
	inputs := make(resource.PropertyMap)

	for name, attr := range attrs {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating attribute %s: %s", name, diags.Error())
		}

		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting attribute %s: %w", name, err)
		}

		inputs[resource.PropertyKey(name)] = pv
	}

	// Build resource options
	opts := e.buildResourceOptions(res, instance)

	// Evaluate preconditions before resource creation
	if len(res.Preconditions) > 0 {
		if err := e.evaluateCheckRules(res.Preconditions, instance.Key, "precondition"); err != nil {
			return err
		}
	}

	// Register the resource
	urn, id, outputs, err := e.registerResource(ctx, typeToken, instance.Key, inputs, opts)
	if err != nil {
		return fmt.Errorf("registering resource: %w", err)
	}

	// Store outputs for future references
	// Convert Pulumi camelCase property names to Terraform snake_case
	outputObj := make(map[string]cty.Value)
	outputObj["id"] = cty.StringVal(id)
	outputObj["urn"] = cty.StringVal(urn)

	for k, v := range outputs {
		// Convert camelCase to snake_case for Terraform compatibility
		snakeKey := camelToSnake(string(k))
		outputObj[snakeKey] = transform.PropertyValueToCty(v)
	}

	e.resourceOutputs[instance.Key] = cty.ObjectVal(outputObj)

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

	// Handle depends_on
	for _, dep := range res.DependsOn {
		depKey := graph.FormatTraversal(dep)
		if depKey != "" {
			opts.DependsOn = append(opts.DependsOn, depKey)
		}
	}

	// Handle lifecycle options
	if res.Lifecycle != nil {
		if res.Lifecycle.PreventDestroy {
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

	// Handle provider reference
	if res.Provider != nil {
		opts.Provider = res.Provider.Name
		if res.Provider.Alias != "" {
			opts.Provider = res.Provider.Name + "." + res.Provider.Alias
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

	// Handle import blocks - resolve import ID from import blocks that target this resource
	opts.ImportId = e.resolveImportId(res)

	return opts
}

// resolveMovedAliases finds any moved blocks that target this resource and returns
// the source addresses as aliases.
func (e *Engine) resolveMovedAliases(res *ast.Resource) []string {
	var aliases []string
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
				aliases = append(aliases, fromAddr)
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
	DependsOn              []string
	Protect                bool
	IgnoreChanges          []string
	Aliases                []string
	Provider               string
	Parent                 string
	DeleteBeforeReplace    bool
	DeleteBeforeReplaceDef bool // True if DeleteBeforeReplace was explicitly set
	CustomTimeouts         *CustomTimeouts
	ImportId               string
}

// registerResource registers a resource with the Pulumi engine.
func (e *Engine) registerResource(
	ctx context.Context,
	typeToken string,
	name string,
	inputs resource.PropertyMap,
	opts *ResourceOptions,
) (string, string, resource.PropertyMap, error) {
	if e.resmon == nil {
		// No resource monitor - return synthetic values for testing
		urn := fmt.Sprintf("urn:pulumi:%s::%s::%s::%s",
			e.stackName, e.projectName, typeToken, name)
		return urn, name, inputs, nil
	}

	// Register with the resource monitor
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:                   typeToken,
		Name:                   name,
		Inputs:                 inputs,
		Dependencies:           opts.DependsOn,
		Protect:                opts.Protect,
		IgnoreChanges:          opts.IgnoreChanges,
		Aliases:                opts.Aliases,
		Provider:               opts.Provider,
		Parent:                 opts.Parent,
		DeleteBeforeReplace:    opts.DeleteBeforeReplace,
		DeleteBeforeReplaceDef: opts.DeleteBeforeReplaceDef,
		CustomTimeouts:         opts.CustomTimeouts,
		ImportId:               opts.ImportId,
	})
	if err != nil {
		return "", "", nil, err
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
	funcType, err := packages.ResolveFunction(ctx, e.pkgLoader, ds.Type)
	if err != nil {
		return fmt.Errorf("resolving data source type %s: %w", ds.Type, err)
	}

	// Evaluate data source configuration
	attrs, _ := ds.Config.JustAttributes()
	inputs := make(resource.PropertyMap)

	for name, attr := range attrs {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating attribute %s: %s", name, diags.Error())
		}

		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting attribute %s: %w", name, err)
		}

		inputs[resource.PropertyKey(name)] = pv
	}

	// Invoke the function
	outputs, err := e.invokeFunction(ctx, funcType.Token, inputs)
	if err != nil {
		return fmt.Errorf("invoking data source: %w", err)
	}

	// Store outputs for future references
	// Convert Pulumi camelCase property names to Terraform snake_case
	outputCty := propertyMapToCtySnakeCase(outputs)
	dsKey := node.Key[5:] // Remove "data." prefix
	e.dataSourceOutputs[dsKey] = outputCty
	e.evaluator.Context().SetDataSource(dsKey, outputCty)

	return nil
}

// invokeFunction invokes a Pulumi function (data source).
func (e *Engine) invokeFunction(
	ctx context.Context,
	funcToken string,
	inputs resource.PropertyMap,
) (resource.PropertyMap, error) {
	if e.resmon == nil {
		// No resource monitor - return empty outputs for testing
		return resource.PropertyMap{}, nil
	}

	// Invoke the function
	resp, err := e.resmon.Invoke(ctx, InvokeRequest{
		Token: funcToken,
		Args:  inputs,
	})
	if err != nil {
		return nil, err
	}

	if len(resp.Failures) > 0 {
		return nil, fmt.Errorf("function invocation failed: %v", resp.Failures)
	}

	return resp.Return, nil
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
		countVal, diags := mod.Count.Value(e.evaluator.Context().HCLContext())
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
		forEachVal, diags := mod.ForEach.Value(e.evaluator.Context().HCLContext())
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
	inputs := make(resource.PropertyMap)
	attrs, _ := mod.Config.JustAttributes()
	for name, attr := range attrs {
		val, diags := attr.Expr.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating module input %s: %s", name, diags.Error())
		}
		pv, err := transform.CtyToPropertyValue(val)
		if err != nil {
			return fmt.Errorf("converting module input %s: %w", name, err)
		}
		inputs[resource.PropertyKey(name)] = pv
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

	componentURN, _, _, err := e.registerComponentResource(ctx, componentType, instance.Key, inputs, componentOpts)
	if err != nil {
		return fmt.Errorf("registering module component: %w", err)
	}

	// Create a child engine to execute the module
	childEngine := e.createChildEngine(loadedModule.Config, componentURN, loadedModule.SourcePath)

	// Set up the child engine's variables with module inputs
	childEngine.setModuleInputs(attrs, e.evaluator.Context())

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
		outputProps := make(resource.PropertyMap)
		if moduleOutputs.Type().IsObjectType() {
			for name, val := range moduleOutputs.AsValueMap() {
				pv, err := transform.CtyToPropertyValue(val)
				if err == nil {
					outputProps[resource.PropertyKey(name)] = pv
				}
			}
		}
		if err := e.resmon.RegisterResourceOutputs(ctx, componentURN, outputProps); err != nil {
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
	inputs resource.PropertyMap,
	opts *ResourceOptions,
) (string, string, resource.PropertyMap, error) {
	if e.resmon == nil {
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
		return "", "", nil, err
	}

	return resp.URN, resp.ID, resp.Outputs, nil
}

// createChildEngine creates a child engine for executing a module.
func (e *Engine) createChildEngine(config *ast.Config, parentURN string, moduleDir string) *Engine {
	return &Engine{
		config: config,
		evaluator: eval.NewEvaluator(eval.NewContext(moduleDir, moduleDir,
			e.stackName, e.projectName, e.organization)),
		pkgLoader:         e.pkgLoader,
		resmon:            e.resmon,
		resourceOutputs:   make(map[string]cty.Value),
		dataSourceOutputs: make(map[string]cty.Value),
		stackOutputs:      make(resource.PropertyMap),
		projectName:       e.projectName,
		stackName:         e.stackName,
		organization:      e.organization,
		dryRun:            e.dryRun,
		workDir:           moduleDir,
		pulumiConfig:      e.pulumiConfig,
		configSecretKeys:  e.configSecretKeys,
		moduleLoader:      e.moduleLoader, // Share the module loader for caching
		moduleOutputs:     make(map[string]cty.Value),
		parentURN:         parentURN,
	}
}

// setModuleInputs sets up the module's variables with input values.
func (e *Engine) setModuleInputs(inputs hcl.Attributes, parentContext *eval.Context) {
	for name, attr := range inputs {
		val, diags := attr.Expr.Value(parentContext.HCLContext())
		if !diags.HasErrors() {
			e.evaluator.Context().SetVariable(name, val)
		}
	}
}

// runModule executes a module's contents (without registering a stack).
func (e *Engine) runModule(ctx context.Context) error {
	// Build dependency graph for the module
	g, err := graph.BuildFromConfig(e.config)
	if err != nil {
		return fmt.Errorf("building module graph: %w", err)
	}

	// Get execution order
	order, err := g.TopologicalSort()
	if err != nil {
		return fmt.Errorf("calculating module execution order: %w", err)
	}

	// Process nodes in order
	for _, node := range order {
		if err := e.processNode(ctx, node); err != nil {
			return fmt.Errorf("processing %s: %w", node.Key, err)
		}
	}

	return nil
}

// collectModuleOutputs collects the module's output values.
func (e *Engine) collectModuleOutputs() cty.Value {
	outputMap := make(map[string]cty.Value)

	for name, output := range e.config.Outputs {
		val, diags := output.Value.Value(e.evaluator.Context().HCLContext())
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
func (e *Engine) processOutput(ctx context.Context, name string, output *ast.Output) error {
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
		pv = resource.MakeSecret(pv)
	}

	// Store the output for later registration on the stack
	e.stackOutputs[resource.PropertyKey(name)] = pv

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

// propertyMapToCtySnakeCase converts a PropertyMap to cty.Value with snake_case keys.
func propertyMapToCtySnakeCase(props resource.PropertyMap) cty.Value {
	if len(props) == 0 {
		return cty.EmptyObjectVal
	}

	result := make(map[string]cty.Value)
	for k, v := range props {
		snakeKey := camelToSnake(string(k))
		result[snakeKey] = transform.PropertyValueToCty(v)
	}
	return cty.ObjectVal(result)
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
		condVal, diags := rule.Condition.Value(e.evaluator.Context().HCLContext())
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
		msgVal, msgDiags := rule.ErrorMessage.Value(e.evaluator.Context().HCLContext())
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
