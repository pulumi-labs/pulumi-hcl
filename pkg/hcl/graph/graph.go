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
	"math/big"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi/pulumi/pkg/v3/util/pdag"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"
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
	NodeTypeVariableValidation
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
	case NodeTypeVariableValidation:
		return "variable_validation"
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

	// successors holds, for each resource node, the nodes that depend on it.
	// Snapshotted from dag.Successors at the end of BuildFromConfig —
	// SpawnInstances needs the set during a Walk, when the dag is being
	// mutated and dag.Successors is not safe to call.
	successors map[string][]pdag.Node

	// instanceDeps maps a dependent node's key to the keys of dependencies it
	// narrows to specific instances: dependency node key → full instance keys
	// (in ExpandedResource.Key form, e.g. `type.name["x"]`). An entry exists
	// only when every reference from the dependent to that dependency is
	// instance-addressed; any whole-resource reference widens the dependency
	// and no entry is recorded.
	instanceDeps map[string]map[string][]string

	// modulePlans holds, per module path, the plan for spawning one walk unit
	// per member node for each module instance (see SpawnModuleInstanceUnits).
	// Computed at the end of BuildFromConfig.
	modulePlans map[modulepath.Path][]*memberPlan

	// initInputEdges are deferred (from, to-init) edges ordering a module
	// call's init after its input expressions' references. Applied
	// best-effort once the whole graph exists, where a cyclic edge (mutual
	// module-input references) can be detected and skipped.
	initInputEdges [][2]pdag.Node

	// moved holds the moved blocks of each module keyed by that module's path.
	// A moved block's from/to addresses are relative to the module it is written
	// in, so resolving a rename needs the blocks scoped to the resource's own
	// module.
	moved map[modulepath.Path][]*ast.Moved

	// scopes maps a module's node-key prefix to its scope, so expression-level
	// dependency extraction (which receives only the prefix) can resolve
	// provider-defined function calls to the provider block they use.
	scopes map[string]*moduleScope

	// missingProviders collects, keyed by provider address, the diagnostics
	// for module-call `providers` entries that name a provider configuration
	// the parent scope does not have. Reported by Validate.
	missingProviders map[string]*hcl.Diagnostic
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
		successors:   make(map[string][]pdag.Node),
		instanceDeps: make(map[string]map[string][]string),
		modulePlans:  make(map[modulepath.Path][]*memberPlan),
		moved:        make(map[modulepath.Path][]*ast.Moved),
		scopes:       make(map[string]*moduleScope),

		missingProviders: make(map[string]*hcl.Diagnostic),
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
	err := g.dag.Walk(ctx, func(ctx context.Context, n dagNode) error {
		if n.exec != nil {
			return n.exec(ctx)
		}
		node, ok := g.seen[n.key]
		contract.Assertf(ok, "invalid graph - key not interned")
		return apply(ctx, node.n)
	}, pdag.MaxProcs(parallel))
	return dropSpuriousCancel(err)
}

// dropSpuriousCancel removes a top-level context.Canceled that the parallel
// walker joins onto a genuine failure. When a node fails, pdag.Walk cancels the
// walk's context and — depending on whether the drain loop is still mid-iteration
// when cancellation fires — non-deterministically joins the resulting
// context.Canceled onto the real error. That trailing cancellation is a
// scheduling artifact, not a distinct failure, so it is dropped whenever a
// genuine error remains; a walk that fails solely because its context was
// canceled still surfaces that.
func dropSpuriousCancel(err error) error {
	joined, ok := err.(interface{ Unwrap() []error })
	if !ok {
		return err
	}
	kept := make([]error, 0, len(joined.Unwrap()))
	for _, e := range joined.Unwrap() {
		if errors.Is(e, context.Canceled) {
			continue
		}
		kept = append(kept, e)
	}
	if len(kept) == 0 {
		return err
	}
	return errors.Join(kept...)
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

// ForcedCreateBeforeDestroy returns the set of resource node keys that must be
// created before their prior instance is destroyed. A resource is included when
// it declares create_before_destroy, or when a resource that transitively
// depends on it does: the behaviour propagates to a resource's dependencies so
// that every create in a replacement chain runs before any delete.
func (g *Graph) ForcedCreateBeforeDestroy() map[string]bool {
	forced := make(map[string]bool)
	visited := make(map[pdag.Node]bool)

	var mark func(node pdag.Node)
	mark = func(node pdag.Node) {
		if visited[node] {
			return
		}
		visited[node] = true
		if key, ok := g.keyByDagNode[node]; ok {
			if n, ok := g.seen[key]; ok && n.n.Type == NodeTypeResource {
				forced[key] = true
			}
		}
		// Recurse into dependencies through any node type, so a dependency
		// reached via a local or other intermediary is still forced.
		for dep := range g.dag.Predecessors(node) {
			mark(dep)
		}
	}

	for _, n := range g.seen {
		if declaresCreateBeforeDestroy(n.n) {
			mark(n.i)
		}
	}
	return forced
}

// declaresCreateBeforeDestroy reports whether a resource node sets
// create_before_destroy = true in its lifecycle block.
func declaresCreateBeforeDestroy(n *Node) bool {
	return n.Type == NodeTypeResource && n.Resource != nil &&
		n.Resource.Lifecycle != nil &&
		n.Resource.Lifecycle.CreateBeforeDestroy != nil &&
		*n.Resource.Lifecycle.CreateBeforeDestroy
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
	rootScope := &moduleScope{config: config}
	g.scopes[""] = rootScope

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
		if err := g.addVariableNodes("var."+name, v, nil, nil, ""); err != nil {
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
		deps := g.providerDeps(provider, key, "")
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
		deps := g.resourceDeps(resource, "", key)
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
		deps := g.resourceDeps(dataSource, "", "data."+key)
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

	// Apply the deferred init-input edges (see initInputEdges), skipping any
	// that the finished graph shows to be cyclic.
	for _, edge := range g.initInputEdges {
		if err := g.dag.NewEdge(edge[0], edge[1]); err != nil {
			var cyc pdag.ErrorCycle[dagNode]
			if errors.As(err, &cyc) {
				continue
			}
			return nil, err
		}
	}

	// Snapshot every resource node's successors for SpawnInstances (see the
	// successors field).
	for key, n := range g.seen {
		if n.n.Type == NodeTypeResource {
			g.successors[key] = slices.Collect(g.dag.Successors(n.i))
		}
	}

	g.buildModulePlans()

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
// mod and parentPrefix describe the module call that instantiated this scope
// (nil/"" for the root), so pass-through provider references can resolve into
// the parent scope.
type moduleScope struct {
	prefix       string
	config       *ast.Config
	parent       *moduleScope
	mod          *ast.Module
	parentPrefix string
}

// inheritedProviderDeps returns an edge to the nearest ancestor's default
// `<pkg>` configuration (see defaultProviderNode) for an in-module
// resource/data source with no `provider`, no own-module block, and no
// pass-through, forcing that configuration to register before the resource.
func (g *Graph) inheritedProviderDeps(resource *ast.Resource, parent *moduleScope) []pdag.Node {
	if resource.Provider != nil {
		return nil
	}
	pkgName := packageNameFromResourceType(resource.Type)
	if pkgName == "" {
		return nil
	}
	return g.defaultProviderNode(pkgName, parent)
}

// passThroughProviderDeps returns an edge from an in-module resource to the
// parent-scope provider that its module call's `providers = { ... }` argument
// passes in for the resource's package. scope is the resource's own module
// scope. Returns nil when no pass-through entry applies.
//
// The reference resolves strictly against the parent scope — provider
// configurations are never inherited into a `providers` argument: the parent
// must have a matching `provider` block, have received the configuration
// through its own module call, or (for an un-aliased reference on the root)
// declare the provider in `required_providers`, which stands for the implicit
// empty default configuration. Anything else is a missing provider, reported
// by Validate.
func (g *Graph) passThroughProviderDeps(resource *ast.Resource, scope *moduleScope) []pdag.Node {
	mod := scope.mod
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
	return g.resolvePassedProvider(scope.config, key, passExpr, scope.parent)
}

// resolvePassedProvider returns an edge to the parent-scope provider
// configuration that a module call's `providers = { <childKey> = <passExpr> }`
// entry names, resolving strictly against parent: its `provider` block, the
// pass-through entry of its own module call (whose shadow node stands in), or
// — for an un-aliased reference on the root — its `required_providers`
// declaration, which stands for the implicit empty default configuration
// (nothing registers it, so there is no node to order after). Anything else
// records a missing-provider diagnostic for Validate. childConfig is the
// called module's config; its `required_providers` names the provider in the
// diagnostic.
func (g *Graph) resolvePassedProvider(
	childConfig *ast.Config, childKey string, passExpr hcl.Expression, parent *moduleScope,
) []pdag.Node {
	parentKey := providerExprKey(passExpr)
	if parentKey == "" {
		return nil
	}
	if _, ok := parent.config.Providers[parentKey]; ok {
		_, idx := g.newNode(parent.prefix + parentKey)
		return []pdag.Node{idx}
	}
	if parent.mod != nil {
		if _, ok := parent.mod.Providers[parentKey]; ok {
			_, idx := g.newNode(parent.prefix + parentKey)
			return []pdag.Node{idx}
		}
	}
	name, alias, aliased := strings.Cut(parentKey, ".")
	if !aliased && parent.parent == nil && parent.config.Terraform != nil {
		if _, ok := parent.config.Terraform.RequiredProviders[name]; ok {
			return nil
		}
	}
	addr := fmt.Sprintf("%sprovider[%q]", parent.prefix, providerFQN(childConfig, strings.SplitN(childKey, ".", 2)[0]))
	if aliased {
		addr += "." + alias
	}
	g.recordMissingProvider(addr, passExpr.Range())
	return nil
}

// recordMissingProvider records a missing-provider diagnostic for Validate,
// deduplicated by provider address.
func (g *Graph) recordMissingProvider(addr string, rng hcl.Range) {
	if _, ok := g.missingProviders[addr]; ok {
		return
	}
	g.missingProviders[addr] = &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("missing provider %s", addr),
		Subject:  rng.Ptr(),
	}
}

// providerFQN returns the fully-qualified address of the provider behind
// localName in config: the `required_providers` source when declared, else the
// default namespace, both anchored at the default registry host.
func providerFQN(config *ast.Config, localName string) string {
	source := localName
	if config.Terraform != nil {
		if req, ok := config.Terraform.RequiredProviders[localName]; ok && req.Source != "" {
			source = req.Source
		}
	}
	switch parts := strings.Split(source, "/"); len(parts) {
	case 1:
		return "registry.opentofu.org/hashicorp/" + parts[0]
	case 2:
		return "registry.opentofu.org/" + source
	default:
		return source
	}
}

// defaultProviderNode returns an edge to the node that registers the default
// (un-aliased) provider configuration `name` as seen from scope s: the
// nearest enclosing scope with a `provider "<name>"` block, or with a
// `providers = { <name> = ... }` entry on its own module call (whose shadow
// node stands in). Returns nil when no configuration exists anywhere — the
// reference then names the implicit empty default configuration, which needs
// no ordering edge.
func (g *Graph) defaultProviderNode(name string, s *moduleScope) []pdag.Node {
	for ; s != nil; s = s.parent {
		if _, ok := s.config.Providers[name]; ok {
			_, idx := g.newNode(s.prefix + name)
			return []pdag.Node{idx}
		}
		if s.mod != nil {
			if _, ok := s.mod.Providers[name]; ok {
				_, idx := g.newNode(s.prefix + name)
				return []pdag.Node{idx}
			}
		}
	}
	return nil
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

// instanceRefs classifies a dependent's resource/data references by instance
// addressing: narrow collects the instance keys of literal-indexed references
// (`a["x"]`, in ExpandedResource.Key form with prefix applied); whole marks
// dependencies referenced as a whole object. A dependency stays narrow only if
// nothing references it as a whole.
type instanceRefs struct {
	whole  map[string]bool
	narrow map[string][]string
}

// record classifies one reference traversal and returns the prefixed
// dependency key ("" for non-reference traversals). A nil receiver only
// resolves the key (for dependents that never narrow).
func (r *instanceRefs) record(traversal hcl.Traversal, prefix string) string {
	dep, instance := InstanceTarget(traversal)
	if dep == "" {
		return ""
	}
	if r == nil {
		return prefix + dep
	}
	if instance == "" {
		r.whole[prefix+dep] = true
	} else {
		r.narrow[prefix+dep] = append(r.narrow[prefix+dep], prefix+instance)
	}
	return prefix + dep
}

// resourceDeps extracts all dependencies from a resource, applying prefix to
// resolved keys. nodeKey is the key the resource's own node is registered
// under; body and depends_on references that address a single count/for_each
// instance are recorded against it in g.instanceDeps.
func (g *Graph) resourceDeps(resource *ast.Resource, prefix, nodeKey string) []pdag.Node {
	seen := make(map[pdag.Node]bool)
	refs := &instanceRefs{whole: make(map[string]bool), narrow: make(map[string][]string)}
	markWhole := func(deps ...pdag.Node) {
		for _, dep := range deps {
			seen[dep] = true
			refs.whole[g.keyByDagNode[dep]] = true
		}
	}

	markWhole(g.exprDeps(resource.Count, prefix)...)
	markWhole(g.exprDeps(resource.ForEach, prefix)...)
	for _, traversal := range resource.DependsOn {
		if dep := refs.record(traversal, prefix); dep != "" {
			g.recordRef(dep, traversal.SourceRange())
			_, idx := g.newNode(dep)
			seen[idx] = true
		}
	}
	if resource.ResourceParent != nil {
		if dep := formatTraversal(resource.ResourceParent); dep != "" {
			g.recordRef(prefix+dep, resource.ResourceParent.SourceRange())
			_, idx := g.newNode(prefix + dep)
			markWhole(idx)
		}
	}
	markWhole(g.exprDeps(resource.Provider, prefix)...)
	// A bare `provider = name` reference (no alias) is a single-segment
	// traversal, which exprDeps drops because it carries no attribute after the
	// root. It names the default configuration, which resolves through
	// inheritance and may be implicit, so order the resource after whichever
	// node registers it (if any). Aliased (`name.alias`) and call-based
	// (`call.x.y`) provider expressions have further segments and are already
	// resolved by exprDeps above.
	if resource.Provider != nil {
		if vars := resource.Provider.Variables(); len(vars) == 1 && len(vars[0]) == 1 {
			markWhole(g.defaultProviderNode(vars[0].RootName(), g.scopes[prefix])...)
		}
	}
	for _, traversal := range resource.Providers {
		if dep := formatTraversal(traversal); dep != "" {
			g.recordRef(prefix+dep, traversal.SourceRange())
			_, idx := g.newNode(prefix + dep)
			markWhole(idx)
		}
	}
	// Body references are classified through refs rather than markWhole, so a
	// literal-indexed reference narrows just like an instance-addressed
	// depends_on.
	if resource.Config != nil {
		for _, dep := range g.bodyDeps(resource.Config, prefix, nil, refs) {
			seen[dep] = true
		}
	}
	if resource.DeletedWith != nil {
		if dep := formatTraversal(resource.DeletedWith); dep != "" {
			g.recordRef(prefix+dep, resource.DeletedWith.SourceRange())
			_, idx := g.newNode(prefix + dep)
			markWhole(idx)
		}
	}
	for _, traversal := range resource.ReplaceWith {
		if dep := formatTraversal(traversal); dep != "" {
			g.recordRef(prefix+dep, traversal.SourceRange())
			_, idx := g.newNode(prefix + dep)
			markWhole(idx)
		}
	}
	if resource.Lifecycle != nil {
		for _, expr := range resource.Lifecycle.ReplaceTriggeredBy {
			markWhole(g.exprDeps(expr, prefix)...)
		}
	}
	markWhole(g.exprDeps(resource.Aliases, prefix)...)

	for dep, instances := range refs.narrow {
		if refs.whole[dep] {
			continue
		}
		if g.instanceDeps[nodeKey] == nil {
			g.instanceDeps[nodeKey] = make(map[string][]string)
		}
		g.instanceDeps[nodeKey][dep] = instances
	}

	return slices.Collect(maps.Keys(seen))
}

// providerDeps extracts all dependencies from a provider block, applying prefix
// to resolved keys. key is the block's own node key: a provider config that
// calls one of its own provider's functions would otherwise depend on itself.
func (g *Graph) providerDeps(provider *ast.Provider, key, prefix string) []pdag.Node {
	seen := make(map[pdag.Node]bool)
	for _, dep := range g.exprDeps(provider.ForEach, prefix) {
		seen[dep] = true
	}
	if provider.Config != nil {
		for _, dep := range g.bodyDeps(provider.Config, prefix, nil, nil) {
			seen[dep] = true
		}
	}
	_, self := g.newNode(key)
	delete(seen, self)
	return slices.Collect(maps.Keys(seen))
}

// bodyDeps extracts dependencies from an HCL body, applying prefix to resolved
// keys. A non-nil refs classifies each resource/data reference for instance
// narrowing.
func (g *Graph) bodyDeps(body hcl.Body, prefix string, exclude map[string]bool, refs *instanceRefs) []pdag.Node {
	if eb, ok := body.(*ast.EscapedBody); ok {
		// Scan the underlying bodies so the native-syntax walk below still
		// sees dynamic and lifecycle blocks; the merged body hides them.
		return append(g.bodyDeps(eb.Base, prefix, exclude, refs), g.bodyDeps(eb.Escape, prefix, exclude, refs)...)
	}

	seen := make(map[pdag.Node]bool)

	attrs, _ := body.JustAttributes()
	for _, attr := range attrs {
		for _, dep := range g.exprDepsExcluding(attr.Expr, prefix, exclude, refs) {
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
				for _, dep := range g.bodyDeps(block.Body, prefix, childExclude, refs) {
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
					for _, dep := range g.exprDepsExcluding(attr.Expr, prefix, exclude, refs) {
						seen[dep] = true
					}
				}
				// Still recurse into nested blocks (precondition, postcondition).
				for _, nested := range block.Body.Blocks {
					for _, dep := range g.bodyDeps(nested.Body, prefix, exclude, refs) {
						seen[dep] = true
					}
				}
			} else {
				for _, dep := range g.bodyDeps(block.Body, prefix, exclude, refs) {
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

// variableValueKeySuffix marks the internal value node of a variable whose
// validation rules were split onto a separate node. "!" cannot appear in an
// HCL identifier, so the key can never collide with a referenceable node.
const variableValueKeySuffix = "!value"

// addVariableNodes adds the graph node(s) for one variable declaration. key is
// the node key consumers reference (prefix + "var." + name); valueDeps are the
// dependencies of evaluating the variable's value (module init and the input
// expression from the calling module block).
//
// A validation rule may reference other objects — say a resource's computed
// output — and the rule must then run after that object, while the variable's
// value must stay available to everything ordered before it (such as nodes on
// the far side of an InjectAfter barrier). Keeping rules with such references
// on the value node would turn that ordering into a cycle, so they are split
// onto a NodeTypeVariableValidation node that owns the public key: consumers
// wait for the checks, and the value itself is evaluated by an internal value
// node that carries a copy of the declaration with the split rules removed.
func (g *Graph) addVariableNodes(key string, v *ast.Variable, modInfo *ModuleInfo, valueDeps []pdag.Node, prefix string) error {
	validationDeps := g.variableValidationDeps(v, key, prefix)
	if len(validationDeps) == 0 {
		return g.AddNode(&Node{
			Key:        key,
			Type:       NodeTypeVariable,
			Variable:   v,
			ModuleInfo: modInfo,
		}, valueDeps)
	}

	valueVar := *v
	valueVar.Validations = nil
	valueKey := key + variableValueKeySuffix
	if err := g.AddNode(&Node{
		Key:        valueKey,
		Type:       NodeTypeVariable,
		Variable:   &valueVar,
		ModuleInfo: modInfo,
	}, valueDeps); err != nil {
		return err
	}
	_, valueIdx := g.newNode(valueKey)
	return g.AddNode(&Node{
		Key:        key,
		Type:       NodeTypeVariableValidation,
		Variable:   v,
		ModuleInfo: modInfo,
	}, append(validationDeps, valueIdx))
}

// exprDeps extracts all dependencies from an expression, applying prefix to resolved keys.
func (g *Graph) exprDeps(expr hcl.Expression, prefix string) []pdag.Node {
	return g.exprDepsExcluding(expr, prefix, nil, nil)
}

func (g *Graph) exprDepsExcluding(expr hcl.Expression, prefix string, exclude map[string]bool, refs *instanceRefs) []pdag.Node {
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
		case "path", "terraform", "count", "each", "self", "pulumi":
			continue
		case "data":
			dep = refs.record(traversal, prefix)
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
			dep = refs.record(traversal, prefix)
		}

		if dep != "" {
			g.recordRef(dep, traversal.SourceRange())
			addToSortedListAsSet(&deps, dep)
		}
	}

	// A provider-defined function call routes through the provider block its
	// namespace resolves to, so order the caller after that block the same way
	// an implicit default-provider reference would.
	for _, providerName := range ast.ProviderFunctionCallsInExpr(expr) {
		if dep, ok := g.providerFunctionDep(prefix, providerName); ok {
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

// providerFunctionDep resolves the provider block a provider-defined function
// call in the scope identified by prefix routes through, mirroring the
// runtime's resolution order: the instantiating module call's pass-through
// providers, then the un-aliased provider block of the call's own module, then
// those of its ancestors. ok is false when no block is declared anywhere — the
// engine then falls back to the package's default provider, which needs no
// ordering edge.
func (g *Graph) providerFunctionDep(prefix, providerName string) (string, bool) {
	for s := g.scopes[prefix]; s != nil; s = s.parent {
		if s.mod != nil {
			if passExpr, ok := s.mod.Providers[providerName]; ok {
				if parentKey := providerExprKey(passExpr); parentKey != "" {
					return s.parentPrefix + parentKey, true
				}
			}
		}
		if _, ok := s.config.Providers[providerName]; ok {
			return s.prefix + providerName, true
		}
	}
	return "", false
}

// FormatTraversal converts a traversal to a dependency string.
// This is exported for use by other packages.
func FormatTraversal(traversal hcl.Traversal) string {
	return formatTraversal(traversal)
}

// InstanceTarget splits a reference traversal into its node-level dependency
// key and, when the traversal addresses a single count/for_each instance
// through a literal index (a dynamic index like `a[each.key]` never survives
// into a traversal), the full instance key in ExpandedResource.Key form
// (`type.name[0]`, `type.name["x"]`). instance is empty for whole-resource
// references and for non-resource targets.
func InstanceTarget(traversal hcl.Traversal) (dep, instance string) {
	dep = formatTraversal(traversal)
	if dep == "" {
		return "", ""
	}
	// The instance index sits right after the resource address: TYPE.NAME[KEY]
	// for managed resources, data.TYPE.NAME[KEY] for data sources. Other
	// namespaces (module, var, ...) have no resource instances.
	i := 2
	switch traversal.RootName() {
	case "data":
		i = 3
	case "var", "local", "path", "terraform", "count", "each", "self", "pulumi", "module", "call":
		return dep, ""
	}
	if i >= len(traversal) {
		return dep, ""
	}
	idx, ok := traversal[i].(hcl.TraverseIndex)
	if !ok {
		return dep, ""
	}
	switch idx.Key.Type() {
	case cty.String:
		return dep, fmt.Sprintf("%s[%q]", dep, idx.Key.AsString())
	case cty.Number:
		if n, acc := idx.Key.AsBigFloat().Int64(); acc == big.Exact {
			return dep, fmt.Sprintf("%s[%d]", dep, n)
		}
	}
	return dep, ""
}

// InstanceExec is one expanded instance of a resource node, scheduled as its
// own walk unit by SpawnInstances.
type InstanceExec struct {
	// Key is the instance key in ExpandedResource.Key form.
	Key  string
	Exec func(context.Context) error
}

// SpawnTarget is a walk node that must be re-gated on a resource's expanded
// instances: a successor of the resource's own node (at root), or a member
// unit and template inside a module clone. Key is the dependent's node key,
// consulted for instance narrowing; an empty Key never narrows (the target
// waits for every instance).
type SpawnTarget struct {
	Key string
	N   pdag.Node
}

// SpawnInstances schedules the expanded instances of resource node nodeKey as
// their own concurrent walk units. It must be called during a Walk from within
// nodeKey's own exec, before it returns: each successor of nodeKey is re-gated
// on the instances it depends on — all of them, unless the successor narrowed
// its references to specific instances — because the node's own completion
// stops implying that its instances have registered.
func (g *Graph) SpawnInstances(nodeKey string, instances []InstanceExec) error {
	succs, ok := g.successors[nodeKey]
	contract.Assertf(ok, "SpawnInstances is only valid for resource nodes, not %q", nodeKey)
	targets := make([]SpawnTarget, len(succs))
	for i, succ := range succs {
		targets[i] = SpawnTarget{Key: g.keyByDagNode[succ], N: succ}
	}
	return g.SpawnInstancesTo(nodeKey, instances, targets)
}

// SpawnInstancesTo is SpawnInstances with an explicit target set, for
// resources whose work runs inside a module-instance unit: the targets are
// that instance's aligned successor units plus the resource's own template.
func (g *Graph) SpawnInstancesTo(nodeKey string, instances []InstanceExec, targets []SpawnTarget) error {
	nodes := make([]pdag.Node, len(instances))
	ready := make([]pdag.Done, len(instances))
	for i, inst := range instances {
		nodes[i], ready[i] = g.dag.NewNode(dagNode{key: inst.Key, exec: inst.Exec})
	}
	for _, t := range targets {
		// nil means the target depends on the whole resource. A narrow key
		// that names no actual instance (a nonexistent instance, or a count
		// index against for_each string keys) also falls back to the whole
		// resource, as OpenTofu's reference resolution does.
		var narrow []string
		if t.Key != "" {
			narrow = g.instanceDeps[t.Key][nodeKey]
		}
		for _, key := range narrow {
			if !slices.ContainsFunc(instances, func(inst InstanceExec) bool { return inst.Key == key }) {
				narrow = nil
				break
			}
		}
		for i, inst := range instances {
			if narrow != nil && !slices.Contains(narrow, inst.Key) {
				continue
			}
			if err := g.dag.NewEdge(nodes[i], t.N); err != nil {
				return err
			}
		}
	}
	for _, done := range ready {
		done()
	}
	return nil
}

// memberPlan is the spawning plan for one member node of a module: the
// classification of its static edges into intra-module (instance-aligned in
// each clone) and external (node-level, unchanged).
type memberPlan struct {
	node *Node
	tmpl pdag.Node
	// sameModulePreds / sameModuleSuccs are member keys of the same module
	// that are static predecessors / successors of this member.
	sameModulePreds []string
	sameModuleSuccs []string
	// externalPreds are the walk nodes of static predecessors owned outside
	// the module: root nodes, or other modules' templates (whose completion
	// implies their units completed).
	externalPreds []pdag.Node
}

// OwnerPath returns the module path whose instances execute this node's work.
// A module's init and completion nodes run per instance of the *enclosing*
// module (they evaluate the call in the parent's scope); every other
// module-inlined node runs per instance of its own module. Root-owned nodes
// run their work synchronously in their exec.
func (n *Node) OwnerPath() modulepath.Path {
	if n.ModuleInfo == nil {
		return modulepath.Root()
	}
	switch n.Type {
	case NodeTypeModuleInit, NodeTypeModule:
		return n.ModuleInfo.ParentPath()
	default:
		return n.ModuleInfo.Path
	}
}

// buildModulePlans classifies every non-root-owned node's static edges for
// SpawnModuleInstanceUnits. Called once at the end of BuildFromConfig.
func (g *Graph) buildModulePlans() {
	// Call nodes execute at root against the root config, and pass-through
	// provider shadows are inert stand-ins with no init edge; neither does
	// per-instance work, so both they and their consumers connect at node
	// level (external).
	planned := func(n *Node) bool {
		return !n.OwnerPath().IsRoot() && n.Type != NodeTypeCall && n.Type != NodeTypeBuiltin
	}

	plans := make(map[string]*memberPlan)
	for key, in := range g.seen {
		if !planned(in.n) {
			continue
		}
		owner := in.n.OwnerPath()
		plan := &memberPlan{node: in.n, tmpl: in.i}
		for pred := range g.dag.Predecessors(in.i) {
			pk := g.keyByDagNode[pred]
			pn, ok := g.seen[pk]
			if ok && planned(pn.n) && pn.n.OwnerPath() == owner {
				if !slices.Contains(plan.sameModulePreds, pk) {
					plan.sameModulePreds = append(plan.sameModulePreds, pk)
				}
			} else if !slices.Contains(plan.externalPreds, pred) {
				plan.externalPreds = append(plan.externalPreds, pred)
			}
		}
		plans[key] = plan
		g.modulePlans[owner] = append(g.modulePlans[owner], plan)
	}
	for key, plan := range plans {
		for _, pk := range plan.sameModulePreds {
			pred := plans[pk]
			pred.sameModuleSuccs = append(pred.sameModuleSuccs, key)
		}
	}
}

// MemberUnit hands a member's walk unit to the exec builder passed to
// SpawnModuleInstanceUnits. Targets is filled in before the unit can run: the
// instance's units of the members that statically depend on this member, plus
// this member's own template — the set a resource unit re-gates when spawning
// its count/for_each instances.
type MemberUnit struct {
	Node    *Node
	Targets []SpawnTarget
	n       pdag.Node
}

// SpawnModuleInstanceUnits creates one concurrent walk unit per member of the
// module at path, for a single module instance. Units carry the module's
// static topology instance-aligned: intra-module edges connect this
// instance's units only, external predecessors attach at node level, and each
// unit becomes a prerequisite of its member's template — so a template
// completes only when every instance's unit has, preserving node-level
// semantics for everything outside the module. Must be called during a Walk
// from within the exec (or unit) of the module's init node.
func (g *Graph) SpawnModuleInstanceUnits(
	path modulepath.Path, label string, exec func(u *MemberUnit) func(context.Context) error,
) error {
	units := make(map[string]*MemberUnit)
	var ready []pdag.Done
	for _, plan := range g.modulePlans[path] {
		u := &MemberUnit{Node: plan.node}
		fn := exec(u)
		var done pdag.Done
		u.n, done = g.dag.NewNode(dagNode{key: plan.node.Key + label, exec: fn})
		units[plan.node.Key] = u
		ready = append(ready, done)
	}
	for _, plan := range g.modulePlans[path] {
		u := units[plan.node.Key]
		for _, pk := range plan.sameModulePreds {
			if err := g.dag.NewEdge(units[pk].n, u.n); err != nil {
				return err
			}
		}
		for _, ext := range plan.externalPreds {
			if err := g.dag.NewEdge(ext, u.n); err != nil {
				return err
			}
		}
		if err := g.dag.NewEdge(u.n, plan.tmpl); err != nil {
			return err
		}
		for _, sk := range plan.sameModuleSuccs {
			u.Targets = append(u.Targets, SpawnTarget{Key: sk, N: units[sk].n})
		}
		u.Targets = append(u.Targets, SpawnTarget{N: plan.tmpl})
	}
	for _, done := range ready {
		done()
	}
	return nil
}

// formatTraversal converts a traversal to a dependency string.
func formatTraversal(traversal hcl.Traversal) string {
	if len(traversal) == 0 {
		return ""
	}

	namespace, parts := eval.ParseTraversal(traversal)

	switch namespace {
	case "var", "local", "path", "terraform", "count", "each", "self", "pulumi":
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

	for _, addr := range slices.Sorted(maps.Keys(g.missingProviders)) {
		errs = append(errs, g.missingProviders[addr])
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
	scope := &moduleScope{
		prefix:       prefix,
		config:       loaded.Config,
		parent:       parent,
		mod:          mod,
		parentPrefix: parentPrefix,
	}
	g.scopes[prefix] = scope
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

	// Init eagerly evaluates the call's input expressions in the parent scope
	// (they become the component resource's registered inputs), so it should
	// order after everything they reference. The edges are deferred to the
	// end of BuildFromConfig and added best-effort there: module inputs may
	// legally reference each other's outputs mutually (acyclic at variable
	// granularity), and those eager component inputs then degrade instead of
	// the graph failing.
	for _, dep := range g.bodyDeps(mod.Config, parentPrefix, nil, nil) {
		g.initInputEdges = append(g.initInputEdges, [2]pdag.Node{dep, initIdx})
	}

	// Variables: each depends on init + the corresponding input expression from the module block.
	moduleInputAttrs, _ := mod.Config.JustAttributes()
	for varName, v := range loaded.Config.Variables {
		varDeps := []pdag.Node{initIdx}
		if inputAttr, ok := moduleInputAttrs[varName]; ok {
			varDeps = append(varDeps, g.exprDeps(inputAttr.Expr, parentPrefix)...)
		}
		if err := g.addVariableNodes(prefix+"var."+varName, v, modInfo, varDeps, prefix); err != nil {
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
		deps := g.providerDeps(provider, prefix+key, prefix)
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
	// (to the parent's provider) happens at runtime via mod.Providers. The
	// shadow depends on the parent-scope configuration it stands for, so that
	// a chain of pass-throughs stays ordered after the originating block —
	// and so every entry is checked against the parent scope, whether or not
	// anything in the child consumes it.
	for localKey, passExpr := range mod.Providers {
		deps := g.resolvePassedProvider(loaded.Config, localKey, passExpr, parent)
		shadowKey := prefix + localKey
		if existing, ok := g.seen[shadowKey]; ok && existing.n.Type != NodeTypeUnknown {
			continue
		}
		if err := g.AddNode(&Node{
			Key:        shadowKey,
			Type:       NodeTypeBuiltin,
			ModuleInfo: modInfo,
		}, deps); err != nil {
			return err
		}
	}

	// Resources
	for key, resource := range loaded.Config.Resources {
		deps := g.resourceDeps(resource, prefix, prefix+key)
		deps = append(deps, initIdx)
		ownDeps := g.defaultProviderDeps(resource, loaded.Config, prefix)
		passDeps := g.passThroughProviderDeps(resource, scope)
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
		deps := g.resourceDeps(ds, prefix, prefix+"data."+key)
		deps = append(deps, initIdx)
		ownDeps := g.defaultProviderDeps(ds, loaded.Config, prefix)
		passDeps := g.passThroughProviderDeps(ds, scope)
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

	// Completion node: depends on init plus everything the module declares —
	// its resources, data sources, outputs, and nested module completions. The
	// completion key is the module call's identifier (without the trailing "."),
	// so that a whole-module reference like `module.<name>` in the parent scope
	// (an output-less `depends_on`, in particular) waits for the module's entire
	// contents, not only the resources that happen to feed an output. Node keys
	// for the nested modules are forward references resolved once they are
	// inlined below.
	completionKey := parentPrefix + "module." + name
	completionDeps := []pdag.Node{initIdx}
	for key := range loaded.Config.Resources {
		_, idx := g.newNode(prefix + key)
		completionDeps = append(completionDeps, idx)
	}
	for key := range loaded.Config.DataSources {
		_, idx := g.newNode(prefix + "data." + key)
		completionDeps = append(completionDeps, idx)
	}
	for outputName := range loaded.Config.Outputs {
		_, idx := g.newNode(prefix + "output." + outputName)
		completionDeps = append(completionDeps, idx)
	}
	for nestedName := range loaded.Config.Modules {
		_, idx := g.newNode(prefix + "module." + nestedName)
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
			deps = append(deps, g.bodyDeps(call.Config, prefix, nil, nil)...)
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
