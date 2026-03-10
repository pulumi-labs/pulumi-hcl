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

// Package graph implements dependency graph construction and topological sorting
// for HCL configuration execution ordering.
package graph

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"maps"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// Node represents a node in the dependency graph.
type Node struct {
	// Key is the unique identifier for this node (e.g., "aws_instance.web" or "local.common_tags")
	Key string

	// Type indicates what kind of node this is
	Type NodeType

	// Resource is set for resource/data nodes
	Resource *ast.Resource

	// Local is set for local value nodes
	Local *ast.Local

	// Variable is set for variable nodes
	Variable *ast.Variable

	// Output is set for output nodes
	Output *ast.Output

	// Module is set for module nodes
	Module *ast.Module

	// Provider is set for provider nodes
	Provider *ast.Provider

	// Call is set for call nodes
	Call *ast.Call
}

// NodeType indicates what type of configuration element a node represents.
type NodeType int

const (
	NodeTypeUnknown NodeType = iota
	NodeTypeVariable
	NodeTypeLocal
	NodeTypeResource
	NodeTypeDataSource
	NodeTypeModule
	NodeTypeOutput
	NodeTypeProvider
	NodeTypeBuiltin
	NodeTypeCall
)

func (t NodeType) String() string {
	switch t {
	case NodeTypeVariable:
		return "variable"
	case NodeTypeLocal:
		return "local"
	case NodeTypeResource:
		return "resource"
	case NodeTypeDataSource:
		return "data"
	case NodeTypeModule:
		return "module"
	case NodeTypeOutput:
		return "output"
	case NodeTypeProvider:
		return "provider"
	case NodeTypeBuiltin:
		return "builtin"
	case NodeTypeCall:
		return "call"
	default:
		return "unknown"
	}
}

// Graph represents a dependency graph of configuration elements.
type Graph struct {
	seen map[string]internedNode
	dag  *pdag.DAG[dagNode]
}

type dagNode struct {
	key  string
	exec func(context.Context) error
}

type internedNode struct {
	i pdag.Node
	n *Node
}

// NewGraph creates a new empty graph.
func NewGraph() *Graph {
	return &Graph{
		seen: make(map[string]internedNode),
		dag:  pdag.New[dagNode](),
	}
}

func (g *Graph) Walk(ctx context.Context, apply func(context.Context, *Node) error, parallel int) error {
	return g.dag.Walk(ctx, func(ctx context.Context, n dagNode) error {
		if n.exec != nil {
			return n.exec(ctx)
		}
		node, ok := g.seen[n.key]
		contract.Assertf(ok, "invalid graph - key not interned")
		return apply(ctx, node.n)
	}, pdag.MaxProcs(parallel))
}

// Inject a step to run after all nodes of kind nodeType, and before any other kind of node.
//
// This creates an inflection point in the graph. All nodes of type nodeType *must* come before all nodes of other
// types.
func (g *Graph) InjectAfter(f func(context.Context) error, nodeType NodeType) error {
	n, done := g.dag.NewNode(dagNode{exec: f})
	done()
	for _, node := range g.seen {
		var err error
		switch node.n.Type {
		case nodeType:
			err = g.dag.NewEdge(node.i, n)
		default:
			err = g.dag.NewEdge(n, node.i)
		}
		if err != nil {
			return err
		}
	}
	return nil
}

func (g *Graph) newNode(key string) (*Node, pdag.Node) {
	if n, ok := g.seen[key]; ok {
		contract.Assertf(n.n.Key == key, "key should not be changed")
		return n.n, n.i
	}
	i, done := g.dag.NewNode(dagNode{key: key})
	n := &Node{Key: key}
	done() // We don't execute the graph as we build - so this is always safe
	g.seen[key] = internedNode{
		i: i,
		n: n,
	}
	return n, i
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node, deps []pdag.Node) error {
	n, i := g.newNode(node.Key)
	*n = *node
	for _, dep := range deps {
		err := g.dag.NewEdge(dep, i)
		if err != nil {
			return err
		}
	}
	return nil
}

// BuildFromConfig builds a dependency graph from an HCL configuration.
func BuildFromConfig(config *ast.Config) (*Graph, error) {
	g := NewGraph()

	contract.AssertNoErrorf(errors.Join(
		g.AddNode(&Node{
			Key:  "pulumi.stack",
			Type: NodeTypeBuiltin,
		}, nil),
		g.AddNode(&Node{
			Key:  "pulumi.project",
			Type: NodeTypeBuiltin,
		}, nil),
		g.AddNode(&Node{
			Key:  "pulumi.organization",
			Type: NodeTypeBuiltin,
		}, nil),
	), "nodes without dependencies cannot error")

	// Add variable nodes (no dependencies, they come from outside)
	for name, v := range config.Variables {
		err := g.AddNode(&Node{
			Key:      "var." + name,
			Type:     NodeTypeVariable,
			Variable: v,
		}, nil)
		if err != nil {
			return nil, err
		}
	}

	// Add local value nodes
	for name, local := range config.Locals {
		deps := g.extractDependenciesFromExpression(local.Value)
		err := g.AddNode(&Node{
			Key:   "local." + name,
			Type:  NodeTypeLocal,
			Local: local,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add provider nodes (must come before resources since resources can reference them)
	for key, provider := range config.Providers {
		deps := g.extractProviderDependencies(provider)
		err := g.AddNode(&Node{
			Key:      key,
			Type:     NodeTypeProvider,
			Provider: provider,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add resource nodes
	for key, resource := range config.Resources {
		deps := g.extractResourceDependencies(resource)
		err := g.AddNode(&Node{
			Key:      key,
			Type:     NodeTypeResource,
			Resource: resource,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add data source nodes
	for key, dataSource := range config.DataSources {
		deps := g.extractResourceDependencies(dataSource)
		err := g.AddNode(&Node{
			Key:      "data." + key,
			Type:     NodeTypeDataSource,
			Resource: dataSource,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add module nodes
	for name, module := range config.Modules {
		deps := g.extractModuleDependencies(module)
		err := g.AddNode(&Node{
			Key:    "module." + name,
			Type:   NodeTypeModule,
			Module: module,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add call nodes (method invocations on resources)
	for key, call := range config.Calls {
		var deps []pdag.Node

		// Depend on the resource or provider being called
		for resKey, res := range config.Resources {
			if res.Name == call.ResourceName {
				_, idx := g.newNode(resKey)
				deps = append(deps, idx)
				break
			}
		}
		if _, exists := config.Providers[call.ResourceName]; exists {
			_, idx := g.newNode(call.ResourceName)
			deps = append(deps, idx)
		}

		// Extract expression deps from args body
		if call.Config != nil {
			deps = append(deps, g.extractDependenciesFromBody(call.Config)...)
		}

		err := g.AddNode(&Node{
			Key:  "call." + key,
			Type: NodeTypeCall,
			Call: call,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Add output nodes
	for name, output := range config.Outputs {
		deps := g.extractDependenciesFromExpression(output.Value)
		err := g.AddNode(&Node{
			Key:    "output." + name,
			Type:   NodeTypeOutput,
			Output: output,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	return g, nil
}

// extractResourceDependencies extracts all dependencies from a resource.
func (g *Graph) extractResourceDependencies(resource *ast.Resource) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	// Extract from count expression
	for _, dep := range g.extractDependenciesFromExpression(resource.Count) {
		seen[dep] = true
	}

	// Extract from for_each expression
	for _, dep := range g.extractDependenciesFromExpression(resource.ForEach) {
		seen[dep] = true
	}

	// Extract from explicit depends_on
	for _, traversal := range resource.DependsOn {
		dep := formatTraversal(traversal)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from parent resource reference
	if resource.ResourceParent != nil {
		dep := formatTraversal(resource.ResourceParent)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from provider reference
	if resource.Provider != nil {
		providerKey := resource.Provider.Name
		if resource.Provider.Alias != "" {
			providerKey = resource.Provider.Name + "." + resource.Provider.Alias
		}
		_, idx := g.newNode(providerKey)
		seen[idx] = true
	}

	// Extract from providers list
	for _, traversal := range resource.Providers {
		dep := formatTraversal(traversal)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from resource body (config block)
	if resource.Config != nil {
		bodyDeps := g.extractDependenciesFromBody(resource.Config)
		for _, dep := range bodyDeps {
			seen[dep] = true
		}
	}

	// Extract from deleted_with (resource reference)
	if resource.DeletedWith != nil {
		dep := formatTraversal(resource.DeletedWith)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from replace_with (list of resource references)
	for _, traversal := range resource.ReplaceWith {
		dep := formatTraversal(traversal)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from replacement_trigger expression
	for _, dep := range g.extractDependenciesFromExpression(resource.ReplacementTrigger) {
		seen[dep] = true
	}

	// Extract from additional_secret_outputs expression
	for _, dep := range g.extractDependenciesFromExpression(resource.AdditionalSecretOutputs) {
		seen[dep] = true
	}

	// Extract from aliases expression
	for _, dep := range g.extractDependenciesFromExpression(resource.Aliases) {
		seen[dep] = true
	}

	return slices.Collect(maps.Keys(seen))
}

// extractModuleDependencies extracts all dependencies from a module block.
func (g *Graph) extractModuleDependencies(module *ast.Module) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	// Extract from count expression
	for _, dep := range g.extractDependenciesFromExpression(module.Count) {
		seen[dep] = true
	}

	// Extract from for_each expression
	for _, dep := range g.extractDependenciesFromExpression(module.ForEach) {
		seen[dep] = true
	}

	// Extract from explicit depends_on
	for _, traversal := range module.DependsOn {
		dep := formatTraversal(traversal)
		if dep != "" {
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}

	// Extract from module config body
	if module.Config != nil {
		bodyDeps := g.extractDependenciesFromBody(module.Config)
		for _, dep := range bodyDeps {
			seen[dep] = true
		}
	}

	return slices.Collect(maps.Keys(seen))
}

// extractProviderDependencies extracts all dependencies from a provider block.
func (g *Graph) extractProviderDependencies(provider *ast.Provider) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	// Extract from provider config body
	if provider.Config != nil {
		bodyDeps := g.extractDependenciesFromBody(provider.Config)
		for _, dep := range bodyDeps {
			seen[dep] = true
		}
	}

	return slices.Collect(maps.Keys(seen))
}

// extractDependenciesFromBody extracts dependencies from an HCL body,
// including attributes and nested blocks (e.g., dynamic blocks).
func (g *Graph) extractDependenciesFromBody(body hcl.Body) []pdag.Node {
	return g.extractDependenciesFromBodyExcluding(body, nil)
}

func (g *Graph) extractDependenciesFromBodyExcluding(body hcl.Body, exclude map[string]bool) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	attrs, _ := body.JustAttributes()
	for _, attr := range attrs {
		for _, dep := range g.extractDependenciesFromExpressionExcluding(attr.Expr, exclude) {
			seen[dep] = true
		}
	}

	if syntaxBody, ok := body.(*hclsyntax.Body); ok {
		for _, block := range syntaxBody.Blocks {
			if block.Type == "dynamic" && len(block.Labels) > 0 {
				// The iterator variable defaults to the block label.
				iterName := block.Labels[0]
				// Check for an explicit "iterator" attribute.
				if iterAttr, ok := block.Body.Attributes["iterator"]; ok {
					if keyword := hcl.ExprAsKeyword(iterAttr.Expr); keyword != "" {
						iterName = keyword
					}
				}
				childExclude := make(map[string]bool, len(exclude)+1)
				for k, v := range exclude {
					childExclude[k] = v
				}
				childExclude[iterName] = true
				for _, dep := range g.extractDependenciesFromBodyExcluding(block.Body, childExclude) {
					seen[dep] = true
				}
			} else {
				for _, dep := range g.extractDependenciesFromBodyExcluding(block.Body, exclude) {
					seen[dep] = true
				}
			}
		}
	}

	return slices.Collect(maps.Keys(seen))
}

// extractDependenciesFromExpression extracts ALL dependencies from an expression,
// including var and local references (unlike eval.ExtractDependencies which only extracts resource deps).
func (g *Graph) extractDependenciesFromExpression(expr hcl.Expression) []pdag.Node {
	return g.extractDependenciesFromExpressionExcluding(expr, nil)
}

func (g *Graph) extractDependenciesFromExpressionExcluding(expr hcl.Expression, exclude map[string]bool) []pdag.Node {
	if expr == nil {
		return nil
	}

	var deps []string

	for _, traversal := range expr.Variables() {
		namespace, parts := eval.ParseTraversal(traversal)

		if exclude[namespace] {
			continue
		}

		var dep string
		switch namespace {
		case "var":
			// Variable reference: var.name
			if len(parts) >= 1 {
				dep = fmt.Sprintf("var.%s", parts[0])
			}
		case "local":
			// Local reference: local.name
			if len(parts) >= 1 {
				dep = fmt.Sprintf("local.%s", parts[0])
			}
		case "path", "terraform", "count", "each", "self":
			// These are not node dependencies
			continue
		case "data":
			// Data source reference: data.type.name
			if len(parts) >= 2 {
				dep = fmt.Sprintf("data.%s.%s", parts[0], parts[1])
			}
		case "module":
			// Module reference: module.name
			if len(parts) >= 1 {
				dep = fmt.Sprintf("module.%s", parts[0])
			}
		case "call":
			// Call reference: call.resourceName.methodName
			if len(parts) >= 2 {
				dep = fmt.Sprintf("call.%s.%s", parts[0], parts[1])
			}
		default:
			// Resource reference: type.name (namespace is the type)
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%s.%s", namespace, parts[0])
			}
		}

		if dep != "" {
			addToSortedListAsSet(&deps, dep)
		}
	}

	result := make([]pdag.Node, len(deps))
	for i, dep := range deps {
		_, n := g.newNode(dep)
		result[i] = n
	}

	return result
}

// FormatTraversal converts a traversal to a dependency string.
// This is exported for use by other packages.
func FormatTraversal(traversal hcl.Traversal) string {
	return formatTraversal(traversal)
}

// formatTraversal converts a traversal to a dependency string.
func formatTraversal(traversal hcl.Traversal) string {
	if len(traversal) == 0 {
		return ""
	}

	namespace, parts := eval.ParseTraversal(traversal)

	switch namespace {
	case "var", "local", "path", "terraform", "count", "each", "self":
		// These are handled differently
		if namespace == "local" && len(parts) >= 1 {
			return "local." + parts[0]
		}
		if namespace == "var" && len(parts) >= 1 {
			return "var." + parts[0]
		}
		return ""
	case "data":
		if len(parts) >= 2 {
			return fmt.Sprintf("data.%s.%s", parts[0], parts[1])
		}
	case "module":
		if len(parts) >= 1 {
			return "module." + parts[0]
		}
	case "call":
		if len(parts) >= 2 {
			return fmt.Sprintf("call.%s.%s", parts[0], parts[1])
		}
	default:
		// Resource reference
		if len(parts) >= 1 {
			return fmt.Sprintf("%s.%s", namespace, parts[0])
		}
	}

	return ""
}

// Validate checks the graph for common issues.
func (g *Graph) Validate() []error {
	var errors []error

	// Check for missing dependencies
	for key, node := range g.seen {
		if node.n.Type == NodeTypeUnknown {
			errors = append(errors, fmt.Errorf("unknown node %q", key))
		}
	}

	return errors
}

func addToSortedListAsSet[S ~[]E, E cmp.Ordered](s *S, element E) {
	idx, found := slices.BinarySearch(*s, element)
	if !found {
		*s = slices.Insert(*s, idx, element)
	}
}
