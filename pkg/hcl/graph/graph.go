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
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// ModuleInfo holds metadata for nodes that are part of an inlined module.
type ModuleInfo struct {
	Prefix       string      // e.g., "module.first." — prefixed to all internal keys
	ModuleName   string      // e.g., "first"
	Module       *ast.Module // the module block from the parent config
	SourcePath   string      // resolved source path (for component type name)
	ParentPrefix string      // "" for root-level modules, "module.outer." for nested
}

// LoadedModule represents a loaded and parsed module (used by ModuleLoader).
type LoadedModule struct {
	Config     *ast.Config
	SourcePath string
}

// ModuleLoader loads module configurations from source paths.
type ModuleLoader interface {
	LoadModule(source string, workDir string) (*LoadedModule, error)
}

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

	// ModuleInfo is set for nodes that belong to an inlined module.
	ModuleInfo *ModuleInfo
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
	NodeTypeModuleInit
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
	case NodeTypeModuleInit:
		return "module_init"
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

// InjectAfter injects a step to run after all nodes matching the predicate, and before any
// other node. This creates an inflection point in the graph.
func (g *Graph) InjectAfter(f func(context.Context) error, match func(*Node) bool) error {
	n, done := g.dag.NewNode(dagNode{exec: f})
	done()
	for _, node := range g.seen {
		var err error
		if match(node.n) {
			err = g.dag.NewEdge(node.i, n)
		} else {
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
// moduleLoader is required when config contains modules.
func BuildFromConfig(config *ast.Config, moduleLoader ModuleLoader, workDir string) (*Graph, error) {
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
		deps := g.exprDeps(local.Value, "")
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
		deps := g.providerDeps(provider, "")
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
		deps := g.resourceDeps(resource, "")
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
		deps := g.resourceDeps(dataSource, "")
		err := g.AddNode(&Node{
			Key:      "data." + key,
			Type:     NodeTypeDataSource,
			Resource: dataSource,
		}, deps)
		if err != nil {
			return nil, err
		}
	}

	// Inline module contents into the graph for fine-grained dependency tracking.
	for name, module := range config.Modules {
		if err := g.inlineModule(name, module, "", moduleLoader, workDir); err != nil {
			return nil, fmt.Errorf("inlining module %s: %w", name, err)
		}
	}

	if err := g.addCallNodes(config, "", nil); err != nil {
		return nil, err
	}

	// Add output nodes
	for name, output := range config.Outputs {
		deps := g.exprDeps(output.Value, "")
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

// resourceDeps extracts all dependencies from a resource, applying prefix to resolved keys.
func (g *Graph) resourceDeps(resource *ast.Resource, prefix string) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	for _, dep := range g.exprDeps(resource.Count, prefix) {
		seen[dep] = true
	}
	for _, dep := range g.exprDeps(resource.ForEach, prefix) {
		seen[dep] = true
	}
	for _, traversal := range resource.DependsOn {
		if dep := formatTraversal(traversal); dep != "" {
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	if resource.ResourceParent != nil {
		if dep := formatTraversal(resource.ResourceParent); dep != "" {
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	if resource.Provider != nil {
		providerKey := resource.Provider.Name
		if resource.Provider.Alias != "" {
			providerKey = resource.Provider.Name + "." + resource.Provider.Alias
		}
		_, idx := g.newNode(prefix + providerKey)
		seen[idx] = true
	}
	for _, traversal := range resource.Providers {
		if dep := formatTraversal(traversal); dep != "" {
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	if resource.Config != nil {
		for _, dep := range g.bodyDeps(resource.Config, prefix, nil) {
			seen[dep] = true
		}
	}
	if resource.DeletedWith != nil {
		if dep := formatTraversal(resource.DeletedWith); dep != "" {
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	for _, traversal := range resource.ReplaceWith {
		if dep := formatTraversal(traversal); dep != "" {
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	for _, dep := range g.exprDeps(resource.ReplacementTrigger, prefix) {
		seen[dep] = true
	}
	for _, dep := range g.exprDeps(resource.AdditionalSecretOutputs, prefix) {
		seen[dep] = true
	}
	for _, dep := range g.exprDeps(resource.Aliases, prefix) {
		seen[dep] = true
	}

	return slices.Collect(maps.Keys(seen))
}

// providerDeps extracts all dependencies from a provider block, applying prefix to resolved keys.
func (g *Graph) providerDeps(provider *ast.Provider, prefix string) []pdag.Node {
	if provider.Config == nil {
		return nil
	}
	return g.bodyDeps(provider.Config, prefix, nil)
}

// bodyDeps extracts dependencies from an HCL body, applying prefix to resolved keys.
func (g *Graph) bodyDeps(body hcl.Body, prefix string, exclude map[string]bool) []pdag.Node {
	seen := make(map[pdag.Node]bool)

	attrs, _ := body.JustAttributes()
	for _, attr := range attrs {
		for _, dep := range g.exprDepsExcluding(attr.Expr, prefix, exclude) {
			seen[dep] = true
		}
	}

	if syntaxBody, ok := body.(*hclsyntax.Body); ok {
		for _, block := range syntaxBody.Blocks {
			if block.Type == "dynamic" && len(block.Labels) > 0 {
				iterName := block.Labels[0]
				if iterAttr, ok := block.Body.Attributes["iterator"]; ok {
					if keyword := hcl.ExprAsKeyword(iterAttr.Expr); keyword != "" {
						iterName = keyword
					}
				}
				childExclude := make(map[string]bool, len(exclude)+1)
				maps.Copy(childExclude, exclude)
				childExclude[iterName] = true
				for _, dep := range g.bodyDeps(block.Body, prefix, childExclude) {
					seen[dep] = true
				}
			} else {
				for _, dep := range g.bodyDeps(block.Body, prefix, exclude) {
					seen[dep] = true
				}
			}
		}
	}

	return slices.Collect(maps.Keys(seen))
}

// exprDeps extracts all dependencies from an expression, applying prefix to resolved keys.
func (g *Graph) exprDeps(expr hcl.Expression, prefix string) []pdag.Node {
	return g.exprDepsExcluding(expr, prefix, nil)
}

func (g *Graph) exprDepsExcluding(expr hcl.Expression, prefix string, exclude map[string]bool) []pdag.Node {
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
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%svar.%s", prefix, parts[0])
			}
		case "local":
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%slocal.%s", prefix, parts[0])
			}
		case "path", "terraform", "count", "each", "self":
			continue
		case "data":
			if len(parts) >= 2 {
				dep = fmt.Sprintf("%sdata.%s.%s", prefix, parts[0], parts[1])
			}
		case "module":
			if len(parts) >= 2 {
				dep = fmt.Sprintf("%smodule.%s.output.%s", prefix, parts[0], parts[1])
			} else if len(parts) >= 1 {
				dep = fmt.Sprintf("%smodule.%s", prefix, parts[0])
			}
		case "call":
			if len(parts) >= 2 {
				dep = fmt.Sprintf("%scall.%s.%s", prefix, parts[0], parts[1])
			}
		default:
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%s%s.%s", prefix, namespace, parts[0])
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

// inlineModule loads a module and inlines its contents into the graph with a prefix.
func (g *Graph) inlineModule(
	name string, mod *ast.Module, parentPrefix string,
	moduleLoader ModuleLoader, workDir string,
) error {
	loaded, err := moduleLoader.LoadModule(mod.Source, workDir)
	if err != nil {
		return fmt.Errorf("loading module %s: %w", name, err)
	}

	prefix := parentPrefix + "module." + name + "."
	modInfo := &ModuleInfo{
		Prefix:       prefix,
		ModuleName:   name,
		Module:       mod,
		SourcePath:   loaded.SourcePath,
		ParentPrefix: parentPrefix,
	}

	// Init node: depends on count/for_each/depends_on from parent scope.
	initKey := prefix + "__init__"
	var initDeps []pdag.Node
	initDeps = append(initDeps, g.exprDeps(mod.Count, parentPrefix)...)
	initDeps = append(initDeps, g.exprDeps(mod.ForEach, parentPrefix)...)
	for _, traversal := range mod.DependsOn {
		if dep := formatTraversal(traversal); dep != "" {
			_, idx := g.newNode(parentPrefix + dep)
			initDeps = append(initDeps, idx)
		}
	}
	if err := g.AddNode(&Node{
		Key:        initKey,
		Type:       NodeTypeModuleInit,
		Module:     mod,
		ModuleInfo: modInfo,
	}, initDeps); err != nil {
		return err
	}

	_, initIdx := g.newNode(initKey)

	// Variables: each depends on init + the corresponding input expression from the module block.
	moduleInputAttrs, _ := mod.Config.JustAttributes()
	for varName, v := range loaded.Config.Variables {
		varDeps := []pdag.Node{initIdx}
		if inputAttr, ok := moduleInputAttrs[varName]; ok {
			varDeps = append(varDeps, g.exprDeps(inputAttr.Expr, parentPrefix)...)
		}
		if err := g.AddNode(&Node{
			Key:        prefix + "var." + varName,
			Type:       NodeTypeVariable,
			Variable:   v,
			ModuleInfo: modInfo,
		}, varDeps); err != nil {
			return err
		}
	}

	// Locals
	for localName, local := range loaded.Config.Locals {
		if err := g.AddNode(&Node{
			Key:        prefix + "local." + localName,
			Type:       NodeTypeLocal,
			Local:      local,
			ModuleInfo: modInfo,
		}, g.exprDeps(local.Value, prefix)); err != nil {
			return err
		}
	}

	// Providers
	for key, provider := range loaded.Config.Providers {
		if err := g.AddNode(&Node{
			Key:        prefix + key,
			Type:       NodeTypeProvider,
			Provider:   provider,
			ModuleInfo: modInfo,
		}, g.providerDeps(provider, prefix)); err != nil {
			return err
		}
	}

	// Resources
	for key, resource := range loaded.Config.Resources {
		deps := g.resourceDeps(resource, prefix)
		deps = append(deps, initIdx)
		if err := g.AddNode(&Node{
			Key:        prefix + key,
			Type:       NodeTypeResource,
			Resource:   resource,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Data sources
	for key, ds := range loaded.Config.DataSources {
		deps := g.resourceDeps(ds, prefix)
		deps = append(deps, initIdx)
		if err := g.AddNode(&Node{
			Key:        prefix + "data." + key,
			Type:       NodeTypeDataSource,
			Resource:   ds,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Outputs
	for outputName, output := range loaded.Config.Outputs {
		if err := g.AddNode(&Node{
			Key:        prefix + "output." + outputName,
			Type:       NodeTypeOutput,
			Output:     output,
			ModuleInfo: modInfo,
		}, g.exprDeps(output.Value, prefix)); err != nil {
			return err
		}
	}

	// Completion node: depends on all outputs + init.
	completionKey := parentPrefix + "module." + name
	completionDeps := []pdag.Node{initIdx}
	for outputName := range loaded.Config.Outputs {
		_, idx := g.newNode(prefix + "output." + outputName)
		completionDeps = append(completionDeps, idx)
	}
	if err := g.AddNode(&Node{
		Key:        completionKey,
		Type:       NodeTypeModule,
		Module:     mod,
		ModuleInfo: modInfo,
	}, completionDeps); err != nil {
		return err
	}

	if err := g.addCallNodes(loaded.Config, prefix, modInfo); err != nil {
		return err
	}

	// Nested modules
	for nestedName, nestedMod := range loaded.Config.Modules {
		if err := g.inlineModule(nestedName, nestedMod, prefix, moduleLoader, loaded.SourcePath); err != nil {
			return fmt.Errorf("inlining nested module %s: %w", nestedName, err)
		}
	}

	return nil
}

// addCallNodes adds call nodes from config into the graph with the given prefix.
func (g *Graph) addCallNodes(config *ast.Config, prefix string, modInfo *ModuleInfo) error {
	for key, call := range config.Calls {
		callKey := prefix + "call." + key
		var deps []pdag.Node
		for resKey, res := range config.Resources {
			if res.Name == call.ResourceName {
				_, idx := g.newNode(prefix + resKey)
				deps = append(deps, idx)
				break
			}
		}
		if _, exists := config.Providers[call.ResourceName]; exists {
			_, idx := g.newNode(prefix + call.ResourceName)
			deps = append(deps, idx)
		}
		if call.Config != nil {
			deps = append(deps, g.bodyDeps(call.Config, prefix, nil)...)
		}
		if err := g.AddNode(&Node{
			Key:        callKey,
			Type:       NodeTypeCall,
			Call:       call,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}
	return nil
}

func addToSortedListAsSet[S ~[]E, E cmp.Ordered](s *S, element E) {
	idx, found := slices.BinarySearch(*s, element)
	if !found {
		*s = slices.Insert(*s, idx, element)
	}
}
