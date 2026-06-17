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
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
)

// ModuleInfo holds metadata for nodes that are part of an inlined module.
type ModuleInfo struct {
	// Path identifies this module call within the nesting tree.
	// Its leaf step is bare (no count/for_each disambiguator) — runtime
	// instances of count/for_each-expanded calls live separately.
	Path modulepath.Path

	Module     *ast.Module // the module block from the parent config
	SourcePath string      // resolved source path (for component type name)

	// ParentSourcePath is the resolved source directory of the parent module
	// (or the root program dir for top-level calls). It's the dir that
	// Module.Source is relative to, so the runtime can re-resolve it when
	// loading the child module on demand.
	ParentSourcePath string
}

// ModuleName returns the block label of this module call (e.g. "first").
func (m *ModuleInfo) ModuleName() string {
	_, last, ok := m.Path.Parent()
	if !ok {
		return ""
	}
	return last.Name()
}

// Prefix returns the string prefix used to disambiguate node keys
// belonging to this module from the rest of the graph.
func (m *ModuleInfo) Prefix() string { return m.Path.PrefixString() }

// ParentPrefix returns the string prefix of the enclosing module, or "" if
// this module is at the root of the configuration.
func (m *ModuleInfo) ParentPrefix() string {
	parent, _, ok := m.Path.Parent()
	if !ok {
		return ""
	}
	return parent.PrefixString()
}

// ParentPath returns the path of the enclosing module, or modulepath.Root() if
// this module is at the root of the configuration.
func (m *ModuleInfo) ParentPath() modulepath.Path {
	parent, _, ok := m.Path.Parent()
	if !ok {
		return modulepath.Root()
	}
	return parent
}

// LoadedModule represents a loaded and parsed module (used by ModuleLoader).
type LoadedModule struct {
	Config     *ast.Config
	SourcePath string
}

// ModuleLoader loads module configurations from source paths. version is
// the `version` attribute on the module block (only meaningful for registry
// sources); workDir is the directory the source is relative to.
type ModuleLoader interface {
	LoadModule(source, version, workDir string) (*LoadedModule, error)
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

	// references records the source location(s) at which each node key was
	// referenced from a user-written traversal. Used by Validate to anchor
	// errors about unknown nodes back at the offending source.
	references map[string][]hcl.Range

	// keyByDagNode lets AddNode resolve each dependency's key so we can
	// track dependent counts. pdag only exposes node indices, not the
	// metadata we attached at creation time.
	keyByDagNode map[pdag.Node]string

	// dependents counts how many other nodes list a given key in their
	// dependency list. Read by HasDependents at Walk time.
	dependents map[string]int

	// moved holds the moved blocks of each module keyed by that module's path.
	// A moved block's from/to addresses are relative to the module it is written
	// in, so resolving a rename needs the blocks scoped to the resource's own
	// module.
	moved map[modulepath.Path][]*ast.Moved
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
		seen:         make(map[string]internedNode),
		dag:          pdag.New[dagNode](),
		references:   make(map[string][]hcl.Range),
		keyByDagNode: make(map[pdag.Node]string),
		dependents:   make(map[string]int),
		moved:        make(map[modulepath.Path][]*ast.Moved),
	}
}

// MovedBlocks returns the moved blocks declared in the module at path (the root
// module is modulepath.Root()). Their from/to addresses are relative to that
// module.
func (g *Graph) MovedBlocks(path modulepath.Path) []*ast.Moved {
	return g.moved[path]
}

// recordRef records that key was referenced from the given source range.
// Multiple references to the same key accumulate; this is used by Validate
// to anchor errors about unknown nodes back to the offending source.
func (g *Graph) recordRef(key string, rng hcl.Range) {
	g.references[key] = append(g.references[key], rng)
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
	g.keyByDagNode[i] = key
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
		if key, ok := g.keyByDagNode[dep]; ok {
			g.dependents[key]++
		}
	}
	return nil
}

// HasDependents reports whether any other node in the graph lists `key` in
// its dependency list. Used by the engine to skip work for nodes whose
// output nothing consumes (e.g. unused `provider` blocks).
func (g *Graph) HasDependents(key string) bool {
	return g.dependents[key] > 0
}

// BuildFromConfig builds a dependency graph from an HCL configuration.
// moduleLoader is required when config contains modules.
func BuildFromConfig(config *ast.Config, moduleLoader ModuleLoader, workDir string) (*Graph, error) {
	g := NewGraph()
	g.moved[modulepath.Root()] = config.Moved

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

	// Variable values come from outside, so a variable depends only on whatever
	// its validation rules reference (e.g. another variable).
	for name, v := range config.Variables {
		err := g.AddNode(&Node{
			Key:      "var." + name,
			Type:     NodeTypeVariable,
			Variable: v,
		}, g.variableValidationDeps(v, "var."+name, ""))
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
		deps = append(deps, g.defaultProviderDeps(resource, config, "")...)
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
		deps = append(deps, g.defaultProviderDeps(dataSource, config, "")...)
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
	rootScope := &moduleScope{config: config}
	for name, module := range config.Modules {
		if err := g.inlineModule(name, module, modulepath.Root(), moduleLoader, workDir, rootScope); err != nil {
			return nil, fmt.Errorf("inlining module %s: %w", name, err)
		}
	}

	if err := g.addCallNodes(config, "", nil); err != nil {
		return nil, err
	}

	// Add output nodes
	for name, output := range config.Outputs {
		deps := g.outputDeps(output, "")
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

// defaultProviderDeps returns an implicit dependency on the un-aliased
// `provider "<pkg>" {}` block, if one exists, for resources/data sources that
// don't set `provider` explicitly. Without this the engine could process the
// resource before the default provider finishes registering — the provider
// block's config would then never make it into the resource.
func (g *Graph) defaultProviderDeps(resource *ast.Resource, config *ast.Config, prefix string) []pdag.Node {
	if resource.Provider != nil {
		return nil
	}
	pkgName := packageNameFromResourceType(resource.Type)
	if pkgName == "" {
		return nil
	}
	if _, ok := config.Providers[pkgName]; !ok {
		return nil
	}
	_, idx := g.newNode(prefix + pkgName)
	return []pdag.Node{idx}
}

// moduleScope is an ancestor module's node-key prefix and config, linked toward
// the root via parent. Scopes are immutable once built and shared by reference.
type moduleScope struct {
	prefix string
	config *ast.Config
	parent *moduleScope
}

// inheritedProviderDeps returns an edge to the nearest ancestor module's
// un-aliased `provider "<pkg>" {}` block, for an in-module resource/data source
// with no `provider`, no own-module block, and no pass-through. parent is the
// enclosing module's scope; the walk runs from there toward the root. The edge
// forces that block to register and orders it before the resource.
func (g *Graph) inheritedProviderDeps(resource *ast.Resource, parent *moduleScope) []pdag.Node {
	if resource.Provider != nil {
		return nil
	}
	pkgName := packageNameFromResourceType(resource.Type)
	if pkgName == "" {
		return nil
	}
	for s := parent; s != nil; s = s.parent {
		if _, ok := s.config.Providers[pkgName]; ok {
			_, idx := g.newNode(s.prefix + pkgName)
			return []pdag.Node{idx}
		}
	}
	return nil
}

// passThroughProviderDeps returns an edge from an in-module resource to the
// parent-scope provider that the module call's `providers = { ... }` argument
// passes in for the resource's package. mod is the module call in the parent
// scope, parentPrefix is the parent scope's prefix. Returns nil when no
// pass-through entry applies.
func (g *Graph) passThroughProviderDeps(resource *ast.Resource, mod *ast.Module, parentPrefix string) []pdag.Node {
	if mod == nil || len(mod.Providers) == 0 {
		return nil
	}
	// Look up by the key the resource would use in the child scope:
	//   explicit `provider = simple.foo` → "simple.foo"
	//   implicit default                  → "simple"
	var key string
	if resource.Provider != nil {
		key = providerExprKey(resource.Provider)
	} else {
		key = packageNameFromResourceType(resource.Type)
	}
	if key == "" {
		return nil
	}
	passExpr, ok := mod.Providers[key]
	if !ok {
		return nil
	}
	parentKey := providerExprKey(passExpr)
	if parentKey == "" {
		return nil
	}
	_, idx := g.newNode(parentPrefix + parentKey)
	return []pdag.Node{idx}
}

// providerExprKey returns "name" or "name.alias" from a provider-reference
// expression. Returns "" for anything that isn't a single one-or-two-step
// traversal.
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

// packageNameFromResourceType mirrors run.packageNameFromResourceType: it
// extracts the provider package from an HCL resource type (e.g.
// "simple_resource" → "simple"). Duplicated here so graph stays free of run
// imports.
func packageNameFromResourceType(token string) string {
	return strings.SplitN(token, "_", 2)[0]
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
			g.recordRef(prefix+dep, traversal.SourceRange())
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	if resource.ResourceParent != nil {
		if dep := formatTraversal(resource.ResourceParent); dep != "" {
			g.recordRef(prefix+dep, resource.ResourceParent.SourceRange())
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	for _, dep := range g.exprDeps(resource.Provider, prefix) {
		seen[dep] = true
	}
	// A bare `provider = name` reference (no alias) is a single-segment
	// traversal, which exprDeps drops because it carries no attribute after the
	// root. Depend on the named provider node directly so this resource is
	// ordered after it. Aliased (`name.alias`) and call-based (`call.x.y`)
	// provider expressions have further segments and are already resolved by
	// exprDeps above.
	if resource.Provider != nil {
		if vars := resource.Provider.Variables(); len(vars) == 1 && len(vars[0]) == 1 {
			name := vars[0].RootName()
			g.recordRef(prefix+name, vars[0].SourceRange())
			_, idx := g.newNode(prefix + name)
			seen[idx] = true
		}
	}
	for _, traversal := range resource.Providers {
		if dep := formatTraversal(traversal); dep != "" {
			g.recordRef(prefix+dep, traversal.SourceRange())
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
			g.recordRef(prefix+dep, resource.DeletedWith.SourceRange())
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	for _, traversal := range resource.ReplaceWith {
		if dep := formatTraversal(traversal); dep != "" {
			g.recordRef(prefix+dep, traversal.SourceRange())
			_, idx := g.newNode(prefix + dep)
			seen[idx] = true
		}
	}
	if resource.Lifecycle != nil {
		for _, expr := range resource.Lifecycle.ReplaceTriggeredBy {
			for _, dep := range g.exprDeps(expr, prefix) {
				seen[dep] = true
			}
		}
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
			} else if block.Type == "lifecycle" {
				// lifecycle blocks contain ignore_changes which holds property
				// paths (e.g. tags["env"]), not dependency references. We must
				// skip that attribute to avoid creating spurious graph nodes.
				for attrName, attr := range block.Body.Attributes {
					if attrName == "ignore_changes" {
						continue
					}
					for _, dep := range g.exprDepsExcluding(attr.Expr, prefix, exclude) {
						seen[dep] = true
					}
				}
				// Still recurse into nested blocks (precondition, postcondition).
				for _, nested := range block.Body.Blocks {
					for _, dep := range g.bodyDeps(nested.Body, prefix, exclude) {
						seen[dep] = true
					}
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

// outputDeps gathers an output node's dependencies: its value expression plus
// the condition and error-message expressions of every precondition, so an
// output is sequenced after everything its precondition references.
func (g *Graph) outputDeps(output *ast.Output, prefix string) []pdag.Node {
	deps := g.exprDeps(output.Value, prefix)
	for _, rule := range output.Preconditions {
		deps = append(deps, g.exprDeps(rule.Condition, prefix)...)
		deps = append(deps, g.exprDeps(rule.ErrorMessage, prefix)...)
	}
	return deps
}

// variableValidationDeps gathers the graph dependencies of a variable's
// `validation` rules — the condition and error-message expressions — minus a
// self-reference to the variable being validated, which is always in scope and
// would otherwise form a cycle.
func (g *Graph) variableValidationDeps(v *ast.Variable, key, prefix string) []pdag.Node {
	_, self := g.newNode(key)
	var deps []pdag.Node
	for _, rule := range v.Validations {
		deps = append(deps, g.exprDeps(rule.Condition, prefix)...)
		deps = append(deps, g.exprDeps(rule.ErrorMessage, prefix)...)
	}
	return slices.DeleteFunc(deps, func(n pdag.Node) bool { return n == self })
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
			if len(parts) >= 1 {
				if out := moduleOutputName(traversal); out != "" {
					dep = fmt.Sprintf("%smodule.%s.output.%s", prefix, parts[0], out)
				} else {
					dep = fmt.Sprintf("%smodule.%s", prefix, parts[0])
				}
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
			g.recordRef(dep, traversal.SourceRange())
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
			if out := moduleOutputName(traversal); out != "" {
				return fmt.Sprintf("module.%s.output.%s", parts[0], out)
			}
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

// moduleOutputName returns the output name from a `module.NAME[idx].OUTPUT`
// traversal (the index step is optional). Returns "" if there's no attribute
// step after the module name, meaning the reference is to the whole module.
func moduleOutputName(traversal hcl.Traversal) string {
	// traversal[0] is the root (`module`), traversal[1] is the module name.
	// The next step is either the output attr, or an index followed by the
	// output attr (for counted / for_each modules).
	i := 2
	if i < len(traversal) {
		if _, ok := traversal[i].(hcl.TraverseIndex); ok {
			i++
		}
	}
	if i < len(traversal) {
		if attr, ok := traversal[i].(hcl.TraverseAttr); ok {
			return attr.Name
		}
	}
	return ""
}

// Validate checks the graph for common issues.
func (g *Graph) Validate() []error {
	var errs []error

	// Sort keys for deterministic error ordering.
	keys := make([]string, 0, len(g.seen))
	for key, node := range g.seen {
		if node.n.Type == NodeTypeUnknown {
			keys = append(keys, key)
		}
	}
	slices.Sort(keys)

	for _, key := range keys {
		diag := &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  fmt.Sprintf("unknown node %q", key),
		}
		if refs := g.references[key]; len(refs) > 0 {
			// Use the earliest reference (sorted by filename, then position)
			// as the subject so the error points at a deterministic location.
			subject := refs[0]
			for _, r := range refs[1:] {
				if rangeLess(r, subject) {
					subject = r
				}
			}
			diag.Subject = subject.Ptr()
		}
		errs = append(errs, diag)
	}

	return errs
}

// rangeLess orders ranges by filename, then start line, then start column.
func rangeLess(a, b hcl.Range) bool {
	if a.Filename != b.Filename {
		return a.Filename < b.Filename
	}
	if a.Start.Line != b.Start.Line {
		return a.Start.Line < b.Start.Line
	}
	return a.Start.Column < b.Start.Column
}

// inlineModule loads a module and inlines its contents into the graph
// rooted at parentPath.
func (g *Graph) inlineModule(
	name string, mod *ast.Module, parentPath modulepath.Path,
	moduleLoader ModuleLoader, workDir string, parent *moduleScope,
) error {
	loaded, err := moduleLoader.LoadModule(mod.Source, mod.Version, workDir)
	if err != nil {
		return fmt.Errorf("loading module %s: %w", name, err)
	}

	path := parentPath.Append(modulepath.NewStep(name))
	prefix := path.PrefixString()
	parentPrefix := parentPath.PrefixString()
	g.moved[path] = loaded.Config.Moved
	modInfo := &ModuleInfo{
		Path:             path,
		Module:           mod,
		SourcePath:       loaded.SourcePath,
		ParentSourcePath: workDir,
	}

	// Init node: depends on count/for_each/depends_on from parent scope,
	// plus the parent module's init (so a nested module never initializes
	// before its enclosing module's instance/eval-context is registered).
	initKey := prefix + "__init__"
	var initDeps []pdag.Node
	if !parentPath.IsRoot() {
		_, parentInitIdx := g.newNode(parentPrefix + "__init__")
		initDeps = append(initDeps, parentInitIdx)
	}
	initDeps = append(initDeps, g.exprDeps(mod.Count, parentPrefix)...)
	initDeps = append(initDeps, g.exprDeps(mod.ForEach, parentPrefix)...)
	for _, traversal := range mod.DependsOn {
		if dep := formatTraversal(traversal); dep != "" {
			g.recordRef(parentPrefix+dep, traversal.SourceRange())
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
		varDeps = append(varDeps, g.variableValidationDeps(v, prefix+"var."+varName, prefix)...)
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
		deps := g.exprDeps(local.Value, prefix)
		deps = append(deps, initIdx)
		if err := g.AddNode(&Node{
			Key:        prefix + "local." + localName,
			Type:       NodeTypeLocal,
			Local:      local,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Providers
	for key, provider := range loaded.Config.Providers {
		deps := g.providerDeps(provider, prefix)
		deps = append(deps, initIdx)
		if err := g.AddNode(&Node{
			Key:        prefix + key,
			Type:       NodeTypeProvider,
			Provider:   provider,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Pass-through aliases: `providers = { simple.foo = simple.from_parent }`
	// creates a local name `simple.foo` in the child that has no in-module
	// `provider` block of its own. In-module resources referencing it would
	// otherwise leave behind an unresolved node and trip Validate. Register
	// it as a no-op shadow so the local edge resolves; the real resolution
	// (to the parent's provider) happens at runtime via mod.Providers.
	for localKey := range mod.Providers {
		shadowKey := prefix + localKey
		if existing, ok := g.seen[shadowKey]; ok && existing.n.Type != NodeTypeUnknown {
			continue
		}
		if err := g.AddNode(&Node{
			Key:        shadowKey,
			Type:       NodeTypeBuiltin,
			ModuleInfo: modInfo,
		}, nil); err != nil {
			return err
		}
	}

	// Resources
	for key, resource := range loaded.Config.Resources {
		deps := g.resourceDeps(resource, prefix)
		deps = append(deps, initIdx)
		ownDeps := g.defaultProviderDeps(resource, loaded.Config, prefix)
		passDeps := g.passThroughProviderDeps(resource, mod, parentPrefix)
		deps = append(deps, ownDeps...)
		deps = append(deps, passDeps...)
		if len(ownDeps) == 0 && len(passDeps) == 0 {
			deps = append(deps, g.inheritedProviderDeps(resource, parent)...)
		}
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
		ownDeps := g.defaultProviderDeps(ds, loaded.Config, prefix)
		passDeps := g.passThroughProviderDeps(ds, mod, parentPrefix)
		deps = append(deps, ownDeps...)
		deps = append(deps, passDeps...)
		if len(ownDeps) == 0 && len(passDeps) == 0 {
			deps = append(deps, g.inheritedProviderDeps(ds, parent)...)
		}
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
		deps := g.outputDeps(output, prefix)
		deps = append(deps, initIdx)
		if err := g.AddNode(&Node{
			Key:        prefix + "output." + outputName,
			Type:       NodeTypeOutput,
			Output:     output,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Completion node: depends on all outputs + init. The completion key
	// is the module call's identifier (without the trailing "."), so that
	// expressions like `module.<name>` in the parent scope resolve to it.
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
	scope := &moduleScope{prefix: prefix, config: loaded.Config, parent: parent}
	for nestedName, nestedMod := range loaded.Config.Modules {
		if err := g.inlineModule(nestedName, nestedMod, path, moduleLoader, loaded.SourcePath, scope); err != nil {
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
