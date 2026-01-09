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
	"fmt"
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/eval"
)

// Node represents a node in the dependency graph.
type Node struct {
	// Key is the unique identifier for this node (e.g., "aws_instance.web" or "local.common_tags")
	Key string

	// Type indicates what kind of node this is
	Type NodeType

	// Dependencies are the keys of nodes this node depends on
	Dependencies []string

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
}

// NodeType indicates what type of configuration element a node represents.
type NodeType int

const (
	NodeTypeVariable NodeType = iota
	NodeTypeLocal
	NodeTypeResource
	NodeTypeDataSource
	NodeTypeModule
	NodeTypeOutput
	NodeTypeProvider
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
	default:
		return "unknown"
	}
}

// Graph represents a dependency graph of configuration elements.
type Graph struct {
	nodes map[string]*Node
}

// NewGraph creates a new empty graph.
func NewGraph() *Graph {
	return &Graph{
		nodes: make(map[string]*Node),
	}
}

// AddNode adds a node to the graph.
func (g *Graph) AddNode(node *Node) {
	g.nodes[node.Key] = node
}

// GetNode returns a node by its key.
func (g *Graph) GetNode(key string) *Node {
	return g.nodes[key]
}

// Nodes returns all nodes in the graph.
func (g *Graph) Nodes() []*Node {
	nodes := make([]*Node, 0, len(g.nodes))
	for _, node := range g.nodes {
		nodes = append(nodes, node)
	}
	return nodes
}

// BuildFromConfig builds a dependency graph from an HCL configuration.
func BuildFromConfig(config *ast.Config) (*Graph, error) {
	g := NewGraph()

	// Add variable nodes (no dependencies, they come from outside)
	for name, v := range config.Variables {
		g.AddNode(&Node{
			Key:          "var." + name,
			Type:         NodeTypeVariable,
			Dependencies: nil,
			Variable:     v,
		})
	}

	// Add local value nodes
	for name, local := range config.Locals {
		deps := extractDependenciesFromExpression(local.Value)
		g.AddNode(&Node{
			Key:          "local." + name,
			Type:         NodeTypeLocal,
			Dependencies: deps,
			Local:        local,
		})
	}

	// Add resource nodes
	for key, resource := range config.Resources {
		deps := extractResourceDependencies(resource)
		g.AddNode(&Node{
			Key:          key,
			Type:         NodeTypeResource,
			Dependencies: deps,
			Resource:     resource,
		})
	}

	// Add data source nodes
	for key, dataSource := range config.DataSources {
		deps := extractResourceDependencies(dataSource)
		g.AddNode(&Node{
			Key:          "data." + key,
			Type:         NodeTypeDataSource,
			Dependencies: deps,
			Resource:     dataSource,
		})
	}

	// Add module nodes
	for name, module := range config.Modules {
		deps := extractModuleDependencies(module)
		g.AddNode(&Node{
			Key:          "module." + name,
			Type:         NodeTypeModule,
			Dependencies: deps,
			Module:       module,
		})
	}

	// Add output nodes
	for name, output := range config.Outputs {
		deps := extractDependenciesFromExpression(output.Value)
		g.AddNode(&Node{
			Key:          "output." + name,
			Type:         NodeTypeOutput,
			Dependencies: deps,
			Output:       output,
		})
	}

	return g, nil
}

// extractResourceDependencies extracts all dependencies from a resource.
func extractResourceDependencies(resource *ast.Resource) []string {
	var deps []string
	seen := make(map[string]bool)

	// Extract from count expression
	for _, dep := range extractDependenciesFromExpression(resource.Count) {
		if !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from for_each expression
	for _, dep := range extractDependenciesFromExpression(resource.ForEach) {
		if !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from explicit depends_on
	for _, traversal := range resource.DependsOn {
		dep := formatTraversal(traversal)
		if dep != "" && !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from resource body (config block)
	if resource.Config != nil {
		bodyDeps := extractDependenciesFromBody(resource.Config)
		for _, dep := range bodyDeps {
			if !seen[dep] {
				deps = append(deps, dep)
				seen[dep] = true
			}
		}
	}

	return deps
}

// extractModuleDependencies extracts all dependencies from a module block.
func extractModuleDependencies(module *ast.Module) []string {
	var deps []string
	seen := make(map[string]bool)

	// Extract from count expression
	for _, dep := range extractDependenciesFromExpression(module.Count) {
		if !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from for_each expression
	for _, dep := range extractDependenciesFromExpression(module.ForEach) {
		if !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from explicit depends_on
	for _, traversal := range module.DependsOn {
		dep := formatTraversal(traversal)
		if dep != "" && !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	// Extract from module config body
	if module.Config != nil {
		bodyDeps := extractDependenciesFromBody(module.Config)
		for _, dep := range bodyDeps {
			if !seen[dep] {
				deps = append(deps, dep)
				seen[dep] = true
			}
		}
	}

	return deps
}

// extractDependenciesFromBody extracts dependencies from an HCL body.
func extractDependenciesFromBody(body hcl.Body) []string {
	var deps []string
	seen := make(map[string]bool)

	attrs, _ := body.JustAttributes()
	for _, attr := range attrs {
		for _, dep := range extractDependenciesFromExpression(attr.Expr) {
			if !seen[dep] {
				deps = append(deps, dep)
				seen[dep] = true
			}
		}
	}

	return deps
}

// extractDependenciesFromExpression extracts ALL dependencies from an expression,
// including var and local references (unlike eval.ExtractDependencies which only extracts resource deps).
func extractDependenciesFromExpression(expr hcl.Expression) []string {
	if expr == nil {
		return nil
	}

	var deps []string
	seen := make(map[string]bool)

	for _, traversal := range expr.Variables() {
		namespace, parts := eval.ParseTraversal(traversal)

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
		default:
			// Resource reference: type.name (namespace is the type)
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%s.%s", namespace, parts[0])
			}
		}

		if dep != "" && !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	return deps
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
	default:
		// Resource reference
		if len(parts) >= 1 {
			return fmt.Sprintf("%s.%s", namespace, parts[0])
		}
	}

	return ""
}

// TopologicalSort performs a topological sort on the graph.
// Returns nodes in execution order (dependencies before dependents).
// Returns an error if a cycle is detected.
func (g *Graph) TopologicalSort() ([]*Node, error) {
	// Create working data structures
	inDegree := make(map[string]int)
	adjacency := make(map[string][]string)

	// Initialize in-degrees and adjacency list
	for key := range g.nodes {
		inDegree[key] = 0
		adjacency[key] = nil
	}

	// Build adjacency list and calculate in-degrees
	for key, node := range g.nodes {
		for _, dep := range node.Dependencies {
			// Only count dependencies that are in the graph
			if _, exists := g.nodes[dep]; exists {
				adjacency[dep] = append(adjacency[dep], key)
				inDegree[key]++
			}
		}
	}

	// Find all nodes with no dependencies
	var queue []string
	for key, degree := range inDegree {
		if degree == 0 {
			queue = append(queue, key)
		}
	}

	// Sort queue for deterministic ordering
	sort.Strings(queue)

	var result []*Node
	for len(queue) > 0 {
		// Take first item from queue
		key := queue[0]
		queue = queue[1:]

		node := g.nodes[key]
		result = append(result, node)

		// Update in-degrees for dependents
		dependents := adjacency[key]
		sort.Strings(dependents) // For deterministic ordering
		for _, dependent := range dependents {
			inDegree[dependent]--
			if inDegree[dependent] == 0 {
				queue = append(queue, dependent)
			}
		}
		sort.Strings(queue) // Maintain sorted order
	}

	// Check for cycle
	if len(result) != len(g.nodes) {
		// Find nodes in cycle
		var cycleNodes []string
		for key, degree := range inDegree {
			if degree > 0 {
				cycleNodes = append(cycleNodes, key)
			}
		}
		return nil, fmt.Errorf("dependency cycle detected involving: %v", cycleNodes)
	}

	return result, nil
}

// ReverseTopologicalSort returns nodes in reverse topological order
// (dependents before dependencies). Useful for destroy ordering.
func (g *Graph) ReverseTopologicalSort() ([]*Node, error) {
	sorted, err := g.TopologicalSort()
	if err != nil {
		return nil, err
	}

	// Reverse the slice
	for i, j := 0, len(sorted)-1; i < j; i, j = i+1, j-1 {
		sorted[i], sorted[j] = sorted[j], sorted[i]
	}

	return sorted, nil
}

// GetDependents returns all nodes that depend on the given node.
func (g *Graph) GetDependents(key string) []*Node {
	var dependents []*Node
	for _, node := range g.nodes {
		for _, dep := range node.Dependencies {
			if dep == key {
				dependents = append(dependents, node)
				break
			}
		}
	}
	return dependents
}

// GetTransitiveDependencies returns all nodes that the given node depends on,
// including indirect dependencies.
func (g *Graph) GetTransitiveDependencies(key string) []*Node {
	visited := make(map[string]bool)
	var result []*Node

	var visit func(k string)
	visit = func(k string) {
		if visited[k] {
			return
		}
		visited[k] = true

		node := g.nodes[k]
		if node == nil {
			return
		}

		for _, dep := range node.Dependencies {
			visit(dep)
		}

		if k != key { // Don't include the starting node
			result = append(result, node)
		}
	}

	visit(key)
	return result
}

// FilterByType returns all nodes of a specific type.
func (g *Graph) FilterByType(nodeType NodeType) []*Node {
	var result []*Node
	for _, node := range g.nodes {
		if node.Type == nodeType {
			result = append(result, node)
		}
	}
	return result
}

// Validate checks the graph for common issues.
func (g *Graph) Validate() []error {
	var errors []error

	// Check for missing dependencies
	for key, node := range g.nodes {
		for _, dep := range node.Dependencies {
			if _, exists := g.nodes[dep]; !exists {
				errors = append(errors, fmt.Errorf("%s depends on unknown %s", key, dep))
			}
		}
	}

	// Check for cycles
	_, err := g.TopologicalSort()
	if err != nil {
		errors = append(errors, err)
	}

	return errors
}
