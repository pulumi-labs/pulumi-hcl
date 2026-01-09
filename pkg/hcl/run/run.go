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
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/graph"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/zclconf/go-cty/cty"
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

// RegisterResourceRequest contains the parameters for registering a resource.
type RegisterResourceRequest struct {
	Type          string
	Name          string
	Inputs        resource.PropertyMap
	Dependencies  []string
	Protect       bool
	IgnoreChanges []string
	Aliases       []string
	Provider      string
	Parent        string
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
	pkgLoader *packages.PackageLoader

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

	// dryRun indicates if this is a preview operation.
	dryRun bool

	// workDir is the working directory.
	workDir string
}

// EngineOptions configures the engine.
type EngineOptions struct {
	// ProjectName is the Pulumi project name.
	ProjectName string

	// StackName is the Pulumi stack name.
	StackName string

	// Config contains the Pulumi configuration values.
	Config map[string]string

	// ConfigSecretKeys lists keys that should be treated as secrets.
	ConfigSecretKeys []string

	// DryRun indicates this is a preview operation.
	DryRun bool

	// ResourceMonitor is the resource monitor for registering resources.
	ResourceMonitor ResourceMonitor

	// WorkDir is the working directory.
	WorkDir string

	// TestMode skips provider/schema validation for unit testing.
	TestMode bool
}

// NewEngine creates a new execution engine.
func NewEngine(config *ast.Config, opts *EngineOptions) *Engine {
	workDir := opts.WorkDir
	if workDir == "" {
		workDir = "."
	}

	pkgLoader := packages.NewPackageLoader()
	if opts.TestMode {
		pkgLoader.SetTestMode(true)
	}

	return &Engine{
		config:            config,
		evaluator:         eval.NewEvaluator(eval.NewContext(workDir)),
		pkgLoader:         pkgLoader,
		resmon:            opts.ResourceMonitor,
		resourceOutputs:   make(map[string]cty.Value),
		dataSourceOutputs: make(map[string]cty.Value),
		stackOutputs:      make(resource.PropertyMap),
		projectName:       opts.ProjectName,
		stackName:         opts.StackName,
		dryRun:            opts.DryRun,
		workDir:           workDir,
	}
}

// Run executes the HCL program.
func (e *Engine) Run(ctx context.Context) error {
	// Register the root stack resource to get its URN for outputs
	if err := e.registerStack(ctx); err != nil {
		return fmt.Errorf("registering stack: %w", err)
	}

	// Preload provider info in parallel for all providers used in the config.
	// This avoids sequential provider invocations during type resolution.
	e.preloadProviders()

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

	// Initialize the evaluation context
	e.evaluator.Context().SetWorkspace(e.stackName)

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

// preloadProviders extracts all unique provider names from the config and
// loads their provider info in parallel. This speeds up type resolution
// by avoiding sequential provider binary invocations.
func (e *Engine) preloadProviders() {
	// Collect unique provider names from resources and data sources
	providers := make(map[string]bool)

	for key := range e.config.Resources {
		// Key format is "type.name", extract the type part
		parts := strings.SplitN(key, ".", 2)
		if len(parts) >= 1 {
			if provider := packages.ExtractProviderFromType(parts[0]); provider != "" {
				providers[provider] = true
			}
		}
	}

	for key := range e.config.DataSources {
		parts := strings.SplitN(key, ".", 2)
		if len(parts) >= 1 {
			if provider := packages.ExtractProviderFromType(parts[0]); provider != "" {
				providers[provider] = true
			}
		}
	}

	// Convert to slice
	var providerList []string
	for p := range providers {
		providerList = append(providerList, p)
	}

	// Preload in parallel
	e.pkgLoader.PreloadProviders(providerList)
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

	// Also send initial signals for nodes with no dependencies
	for _, node := range nodes {
		if pendingDeps[node.Key] == 0 {
			select {
			case done <- "":
			default:
			}
		}
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

	// Get variable value from config or use default
	var val cty.Value

	// TODO: Get from Pulumi config
	// For now, use the default value if available
	if v.Default != nil {
		var diags hcl.Diagnostics
		val, diags = v.Default.Value(e.evaluator.Context().HCLContext())
		if diags.HasErrors() {
			return fmt.Errorf("evaluating variable default: %s", diags.Error())
		}
	} else {
		// Variable has no default and no config value - this is an error in strict mode
		// For now, use null
		val = cty.NullVal(cty.DynamicPseudoType)
	}

	// Store in eval context
	varName := node.Key[4:] // Remove "var." prefix
	e.evaluator.Context().SetVariable(varName, val)

	return nil
}

// processLocal processes a local value definition.
func (e *Engine) processLocal(ctx context.Context, node *graph.Node) error {
	local := node.Local
	if local == nil {
		return fmt.Errorf("local node missing Local field")
	}

	// Evaluate the local value expression
	val, diags := local.Value.Value(e.evaluator.Context().HCLContext())
	if diags.HasErrors() {
		return fmt.Errorf("evaluating local value: %s", diags.Error())
	}

	// Store in eval context
	localName := node.Key[6:] // Remove "local." prefix
	e.evaluator.Context().SetLocal(localName, val)

	return nil
}

// processResource processes a resource definition.
func (e *Engine) processResource(ctx context.Context, node *graph.Node) error {
	res := node.Resource
	if res == nil {
		return fmt.Errorf("resource node missing Resource field")
	}

	// Resolve the resource type to a Pulumi type token
	typeToken, err := e.pkgLoader.ResolveResourceType(res.Type)
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
		// Aliases
		opts.Aliases = res.Lifecycle.Aliases
	}

	// Handle provider reference
	if res.Provider != nil {
		opts.Provider = res.Provider.Name
		if res.Provider.Alias != "" {
			opts.Provider = res.Provider.Name + "." + res.Provider.Alias
		}
	}

	return opts
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
	DependsOn     []string
	Protect       bool
	IgnoreChanges []string
	Aliases       []string
	Provider      string
	Parent        string
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

	// Build dependencies list
	deps := opts.DependsOn

	// Register with the resource monitor
	resp, err := e.resmon.RegisterResource(ctx, RegisterResourceRequest{
		Type:          typeToken,
		Name:          name,
		Inputs:        inputs,
		Dependencies:  deps,
		Protect:       opts.Protect,
		IgnoreChanges: opts.IgnoreChanges,
		Aliases:       opts.Aliases,
		Provider:      opts.Provider,
		Parent:        opts.Parent,
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
	funcToken, err := e.pkgLoader.ResolveDataSourceType(ds.Type)
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
	outputs, err := e.invokeFunction(ctx, funcToken, inputs)
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
func (e *Engine) processModule(ctx context.Context, node *graph.Node) error {
	mod := node.Module
	if mod == nil {
		return fmt.Errorf("module node missing Module field")
	}

	// For now, modules map to Pulumi components
	// This is a simplified implementation - full support requires:
	// 1. Loading the module source (local path or remote package)
	// 2. Parsing and executing the module's HCL
	// 3. Passing inputs and collecting outputs

	// TODO: Implement full module support
	return fmt.Errorf("module support not yet implemented")
}

// processOutput processes an output definition.
func (e *Engine) processOutput(ctx context.Context, name string, output *ast.Output) error {
	// Evaluate the output value
	val, diags := output.Value.Value(e.evaluator.Context().HCLContext())
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
