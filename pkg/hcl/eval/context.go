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

package eval

import (
	"fmt"
	"maps"
	"path/filepath"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// splitResourceKey splits a resource key like "aws_instance.web" into ["aws_instance", "web"].
func splitResourceKey(key string) []string {
	// Find the first dot to split type.name — resource types never contain dots,
	// but logical names can.
	before, after, found := strings.Cut(key, ".")
	if !found {
		return []string{key}
	}
	return []string{before, after}
}

// Context manages the evaluation context for HCL expressions.
// It tracks variables, locals, resources, and other values that can be referenced.
type Context struct {
	// mu protects concurrent access to maps. A pointer so a WithIteration view
	// shares the lock with the context it derives from.
	mu *sync.RWMutex

	// rootModuleDir is the directory of the root module (the program files).
	rootModuleDir string

	// variables contains input variable values (var.*)
	variables map[string]cty.Value

	// locals contains local value results (local.*)
	locals map[string]cty.Value

	// resources contains resource outputs (resource_type.name.*)
	resources map[string]cty.Value

	// rangedResources contains resource instances from count/for_each expansion,
	// keyed by the base resource key (e.g., "type.name").
	rangedResources map[string][]rangedInstance

	// dataSources contains data source outputs (data.type.name.*)
	dataSources map[string]cty.Value

	// modules contains module outputs (module.name.*)
	modules map[string]cty.Value

	// moduleOutputs stores incremental per-output values: moduleName → outputName → value
	moduleOutputs map[string]map[string]cty.Value

	// calls contains call results keyed as "resourceName.methodName"
	calls map[string]cty.Value

	// path contains path information
	path PathContext

	// pulumi contains pulumi metadata
	pulumi PulumiContext

	// count contains count context for count iteration
	count *CountContext

	// each contains each context for for_each iteration
	each *EachContext

	// typeInference makes HCLContext serve type-preserving function variants
	// (see TypeInferenceFunctions). It is set only when the context is used to
	// infer output/local types from unknown values during schema generation.
	typeInference bool

	// providerFunctions holds provider-defined functions callable as
	// provider::<localname>::<name>(...), merged into the function table
	// HCLContext serves. Keys are full table keys (see ast.ProviderFunctionName).
	providerFunctions map[string]function.Function
}

// PathContext contains path-related values.
type PathContext struct {
	// Module is the path to the current module
	Module string
	// Root is the path to the root module
	Root string
	// Cwd is the current working directory
	Cwd string
}

// PulumiContext contains pulumi metadata.
type PulumiContext struct {
	// Stack is the current stack name
	Stack string
	// Project is the current project name
	Project string
	// Organization is the current organization name
	Organization string
	// ModuleName is the resolved Pulumi logical name of the module instance
	// this context evaluates, exposed as pulumi.module.name. Nil at the root,
	// where pulumi.module.name is null.
	ModuleName *string
}

// CountContext contains count iteration context.
type CountContext struct {
	// Index is the current iteration index (count.index)
	Index int
}

// EachContext contains for_each iteration context.
type EachContext struct {
	// Key is the current iteration key (each.key)
	Key cty.Value
	// Value is the current iteration value (each.value)
	Value cty.Value
}

// rangedInstance represents a single instance of a resource expanded via count or for_each.
type rangedInstance struct {
	value   cty.Value
	index   int    // used when isCount is true
	eachKey string // used when isEach is true
	isCount bool
	isEach  bool
}

// NewContext creates a new evaluation context from its three governing
// directories:
//
//   - moduleDir:     the source dir of the module owning this context, used to
//     derive path.module.
//   - rootDir:       the dir path.cwd is taken from.
//   - rootModuleDir: the dir of the root module (the program files), against
//     which file functions resolve relative paths. It stays the same across
//     child-module contexts so that a `"${path.module}/<file>"` argument
//     locates the file the way OpenTofu does, rather than one directory too
//     deep. For the root context it is the same as moduleDir.
//
// Those determine the Terraform-compatible path.* triple:
//
//   - path.module: moduleDir relative to rootDir (or "." when they're the same)
//   - path.root:   always "." — the root module is the program dir
//   - path.cwd:    rootDir as an absolute path. Terraform's path.cwd is the
//     original working dir before any -chdir, which for `tofu apply` from
//     a project dir is that same dir. The language host process's own
//     cwd is irrelevant — it's wherever Pulumi launched the binary, not
//     where the user invoked `pulumi`.
//
// We match Terraform's convention so .tf written against Terraform sees the
// same values when run via Pulumi.
func NewContext(moduleDir, rootDir, rootModuleDir, stack, project, organization string) (*Context, error) {
	modulePath := "."
	if moduleDir != rootDir {
		rel, err := filepath.Rel(rootDir, moduleDir)
		if err != nil {
			return nil, fmt.Errorf("computing %q relative to %q: %w", moduleDir, rootDir, err)
		}
		modulePath = rel
	}
	cwd, err := filepath.Abs(rootDir)
	if err != nil {
		return nil, fmt.Errorf("computing the absolute path of %q: %w", rootDir, err)
	}
	return newContext(PathContext{
		Module: modulePath,
		Root:   ".",
		Cwd:    cwd,
	}, rootModuleDir, stack, project, organization), nil
}

// NewAbsolutePathContext is NewContext for engine runs whose root module lives
// outside the Pulumi program directory — a module consumed as a component or a
// parameterized package. Provider plugins resolve relative paths against the
// program directory, not the module tree, so a relative path.module handed to
// a provider (say, a local_file data source's filename) would point outside
// the module. Here the path.* triple is the directories themselves:
//
//   - path.module: moduleDir
//   - path.root:   rootModuleDir
//   - path.cwd:    rootDir
//
// All three must be absolute, as the module loader resolves them.
func NewAbsolutePathContext(moduleDir, rootDir, rootModuleDir, stack, project, organization string) (*Context, error) {
	for _, dir := range []string{moduleDir, rootDir, rootModuleDir} {
		if !filepath.IsAbs(dir) {
			return nil, fmt.Errorf("%q must be an absolute path", dir)
		}
	}
	return newContext(PathContext{
		Module: moduleDir,
		Root:   rootModuleDir,
		Cwd:    rootDir,
	}, rootModuleDir, stack, project, organization), nil
}

func newContext(path PathContext, rootModuleDir, stack, project, organization string) *Context {
	return &Context{
		mu:              new(sync.RWMutex),
		rootModuleDir:   rootModuleDir,
		variables:       make(map[string]cty.Value),
		locals:          make(map[string]cty.Value),
		resources:       make(map[string]cty.Value),
		rangedResources: make(map[string][]rangedInstance),
		dataSources:     make(map[string]cty.Value),
		modules:         make(map[string]cty.Value),
		moduleOutputs:   make(map[string]map[string]cty.Value),
		calls:           make(map[string]cty.Value),
		path:            path,
		pulumi: PulumiContext{
			Stack:        stack,
			Project:      project,
			Organization: organization,
		},
	}
}

// SetVariable sets an input variable value.
func (c *Context) SetVariable(name string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.variables[name] = value
}

// SetLocal sets a local value.
func (c *Context) SetLocal(name string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.locals[name] = value
}

type resourceMark struct {
	urn     urn.URN
	resHash int
}

// MarkResourceReference tags value as a reference to the resource identified by
// urn: it strips synthetic attributes and applies a resourceMark whose hash
// matches the resulting value. ResourceReferenceURN recovers the URN, and the
// value's own attributes (its outputs) are read transparently — so a reference
// to a resource and the resource itself are the same kind of value.
func MarkResourceReference(value cty.Value, urn urn.URN) cty.Value {
	r := stripSyntheticAttributes(value)
	hashable, _ := r.UnmarkDeep()
	return r.Mark(resourceMark{urn, hashable.Hash()})
}

// ResourceReferenceURN reports the URN val refers to when val is a
// whole-resource reference.
func ResourceReferenceURN(val cty.Value) (urn.URN, bool) {
	if !val.IsKnown() {
		return "", false
	}
	_, marks := val.Unmark()
	hashable, _ := val.UnmarkDeep()
	hash := hashable.Hash()
	for m := range marks {
		if rm, ok := m.(resourceMark); ok && rm.resHash == hash {
			return rm.urn, true
		}
	}
	return "", false
}

// SetResource sets a resource's output values.
// The key should be "type.name" (e.g., "aws_instance.web").
func (c *Context) SetResource(key string, urn urn.URN, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resources[key] = MarkResourceReference(value, urn)
}

// SetCountResource stores a resource instance from count expansion.
// baseKey is the resource key without the index suffix (e.g., "type.name").
func (c *Context) SetCountResource(baseKey string, index int, urn urn.URN, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rangedResources[baseKey] = append(c.rangedResources[baseKey], rangedInstance{
		value: MarkResourceReference(value, urn), index: index, isCount: true,
	})
}

// SetEachResource stores a resource instance from for_each expansion.
// baseKey is the resource key without the each key suffix (e.g., "type.name").
func (c *Context) SetEachResource(baseKey string, eachKey string, urn urn.URN, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rangedResources[baseKey] = append(c.rangedResources[baseKey], rangedInstance{
		value: MarkResourceReference(value, urn), eachKey: eachKey, isEach: true,
	})
}

// SetDataSource sets a data source's output values.
// The key should be "type.name" (e.g., "aws_ami.ubuntu").
func (c *Context) SetDataSource(key string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.dataSources[key] = value
}

// SetModule sets a module's output values.
func (c *Context) SetModule(name string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.modules[name] = value
}

// SetModuleOutput stores a single module output incrementally.
// These are assembled into module namespace objects in HCLContext().
func (c *Context) SetModuleOutput(moduleName, outputName string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.moduleOutputs[moduleName] == nil {
		c.moduleOutputs[moduleName] = make(map[string]cty.Value)
	}
	c.moduleOutputs[moduleName][outputName] = value
}

// SetCall sets the result of a method call.
// The key should be "resourceName.methodName".
func (c *Context) SetCall(key string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.calls[key] = value
}

// WithIteration returns a view of c that shares its evaluation state and lock
// but carries its own count.index / each.key / each.value, so concurrent
// registrations bind the iteration goroutine-locally instead of racing on the
// shared context. A nil index or nil each pair leaves that namespace unbound.
func (c *Context) WithIteration(index *int, eachKey, eachValue *cty.Value) *Context {
	view := *c
	view.count = nil
	view.each = nil
	if index != nil {
		view.count = &CountContext{Index: *index}
	}
	if eachKey != nil && eachValue != nil {
		view.each = &EachContext{Key: *eachKey, Value: *eachValue}
	}
	return &view
}

// SetCount sets the count context for count iteration.
func (c *Context) SetCount(index int) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.count = &CountContext{Index: index}
}

// SetEach sets the each context for for_each iteration.
func (c *Context) SetEach(key, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.each = &EachContext{Key: key, Value: value}
}

// SetModuleName records the resolved Pulumi logical name of the module
// instance this context evaluates, exposed as pulumi.module.name.
func (c *Context) SetModuleName(name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pulumi.ModuleName = &name
}

// HCLContext returns an hcl.EvalContext for evaluating expressions.
func (c *Context) HCLContext() *hcl.EvalContext {
	c.mu.RLock()
	defer c.mu.RUnlock()

	vars := make(map[string]cty.Value)

	// Add var.* namespace
	if len(c.variables) > 0 {
		vars["var"] = cty.ObjectVal(c.variables)
	} else {
		vars["var"] = cty.EmptyObjectVal
	}

	// Add local.* namespace
	if len(c.locals) > 0 {
		vars["local"] = cty.ObjectVal(c.locals)
	} else {
		vars["local"] = cty.EmptyObjectVal
	}

	// Add resource references (resource_type.name.*)
	// Resources are referenced directly by type.name, not under a "resource" namespace.
	// Single instances go into c.resources; ranged instances (count/for_each) go into
	// c.rangedResources and are assembled into tuples/objects here so HCL indexing works:
	//   type.name[0].attr  -> tuple index
	//   type.name["k"].attr -> object key
	resourcesByType := make(map[string]map[string]cty.Value)
	for key, value := range c.resources {
		parts := splitResourceKey(key)
		if len(parts) == 2 {
			typeName, resName := parts[0], parts[1]
			if resourcesByType[typeName] == nil {
				resourcesByType[typeName] = make(map[string]cty.Value)
			}
			resourcesByType[typeName][resName] = value
		} else {
			vars[key] = value
		}
	}

	// Assemble ranged resource instances into tuples (count) or objects (for_each).
	for baseKey, instances := range c.rangedResources {
		parts := splitResourceKey(baseKey)
		if len(parts) != 2 {
			continue
		}
		typeName, resName := parts[0], parts[1]
		if resourcesByType[typeName] == nil {
			resourcesByType[typeName] = make(map[string]cty.Value)
		}
		if len(instances) > 0 && instances[0].isCount {
			// Instances register concurrently, so lower indexes may not be in
			// yet when a dependent narrowed to one index evaluates. Keep every
			// index at its true position, holding unregistered gaps unknown —
			// a correctly scheduled dependent never reads a gap.
			maxIndex := 0
			for _, inst := range instances {
				maxIndex = max(maxIndex, inst.index)
			}
			vals := make([]cty.Value, maxIndex+1)
			for i := range vals {
				vals[i] = cty.DynamicVal
			}
			for _, inst := range instances {
				vals[inst.index] = inst.value
			}
			resourcesByType[typeName][resName] = cty.TupleVal(vals)
		} else {
			objMap := make(map[string]cty.Value, len(instances))
			for _, inst := range instances {
				objMap[inst.eachKey] = inst.value
			}
			resourcesByType[typeName][resName] = cty.ObjectVal(objMap)
		}
	}

	for typeName, instances := range resourcesByType {
		vars[typeName] = cty.ObjectVal(instances)
	}

	// Add data.* namespace for data sources
	// Data sources are referenced as data.type.name.attr
	if len(c.dataSources) > 0 {
		// Group by type: data.aws_ami.ubuntu -> data["aws_ami"]["ubuntu"]
		typeGroups := make(map[string]map[string]cty.Value)
		for key, value := range c.dataSources {
			parts := splitResourceKey(key)
			if len(parts) == 2 {
				typeName, dsName := parts[0], parts[1]
				if typeGroups[typeName] == nil {
					typeGroups[typeName] = make(map[string]cty.Value)
				}
				typeGroups[typeName][dsName] = value
			}
		}
		dataMap := make(map[string]cty.Value)
		for typeName, instances := range typeGroups {
			dataMap[typeName] = cty.ObjectVal(instances)
		}
		if len(dataMap) > 0 {
			vars["data"] = cty.ObjectVal(dataMap)
		} else {
			vars["data"] = cty.EmptyObjectVal
		}
	} else {
		vars["data"] = cty.EmptyObjectVal
	}

	// Add module.* namespace — merge whole-module values and incremental outputs.
	mergedModules := maps.Clone(c.modules)
	for modName, outputs := range c.moduleOutputs {
		if _, alreadySet := mergedModules[modName]; !alreadySet {
			if len(outputs) > 0 {
				mergedModules[modName] = cty.ObjectVal(outputs)
			} else {
				mergedModules[modName] = cty.EmptyObjectVal
			}
		}
	}
	if len(mergedModules) > 0 {
		vars["module"] = cty.ObjectVal(mergedModules)
	} else {
		vars["module"] = cty.EmptyObjectVal
	}

	// Add call.* namespace for method call results
	// Calls are referenced as call.resourceName.methodName.attr
	callsByResource := make(map[string]map[string]cty.Value)
	for key, value := range c.calls {
		parts := splitResourceKey(key)
		if len(parts) == 2 {
			if callsByResource[parts[0]] == nil {
				callsByResource[parts[0]] = make(map[string]cty.Value)
			}
			callsByResource[parts[0]][parts[1]] = value
		}
	}
	if len(callsByResource) > 0 {
		callMap := make(map[string]cty.Value)
		for rName, methods := range callsByResource {
			callMap[rName] = cty.ObjectVal(methods)
		}
		vars["call"] = cty.ObjectVal(callMap)
	} else {
		vars["call"] = cty.EmptyObjectVal
	}

	// Add path.* namespace
	vars["path"] = cty.ObjectVal(map[string]cty.Value{
		"module": cty.StringVal(c.path.Module),
		"root":   cty.StringVal(c.path.Root),
		"cwd":    cty.StringVal(c.path.Cwd),
	})

	// Add pulumi.* namespace
	moduleName := cty.NullVal(cty.String)
	if c.pulumi.ModuleName != nil {
		moduleName = cty.StringVal(*c.pulumi.ModuleName)
	}
	vars["pulumi"] = cty.ObjectVal(map[string]cty.Value{
		"stack":        cty.StringVal(c.pulumi.Stack),
		"project":      cty.StringVal(c.pulumi.Project),
		"organization": cty.StringVal(c.pulumi.Organization),
		"module":       cty.ObjectVal(map[string]cty.Value{"name": moduleName}),
	})

	// terraform.workspace is the stack name: a Pulumi stack, like a Terraform
	// workspace, selects a named state instance of the same program.
	vars["terraform"] = cty.ObjectVal(map[string]cty.Value{
		"workspace": cty.StringVal(c.pulumi.Stack),
	})

	// Add count.* if in count context
	if c.count != nil {
		vars["count"] = cty.ObjectVal(map[string]cty.Value{
			"index": cty.NumberIntVal(int64(c.count.Index)),
		})
	}

	// Add each.* if in for_each context
	if c.each != nil {
		vars["each"] = cty.ObjectVal(map[string]cty.Value{
			"key":   c.each.Key,
			"value": c.each.Value,
		})
	}

	functions := Functions(c.rootModuleDir)
	if c.typeInference {
		functions = TypeInferenceFunctions(c.rootModuleDir)
	}
	maps.Copy(functions, c.providerFunctions)
	return &hcl.EvalContext{
		Variables: vars,
		Functions: functions,
	}
}

// SetProviderFunctions installs the provider-defined function table served by
// HCLContext. Keys must be full table keys (see ast.ProviderFunctionName).
func (c *Context) SetProviderFunctions(functions map[string]function.Function) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providerFunctions = functions
}

// UseTypeInferenceFunctions switches the context to type-preserving function
// variants, for inferring types from unknown values during schema generation.
// See TypeInferenceFunctions.
func (c *Context) UseTypeInferenceFunctions() {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.typeInference = true
}

// HCLContextWithIteration returns an hcl.EvalContext like HCLContext, but with
// count.index and/or each.key/each.value bound to the supplied iteration
// values. Callers pass nil for either argument to leave that namespace unset.
// Useful for evaluating an expression in a parent scope (e.g. a module input)
// while binding it to a specific iteration of a counted / for_each call.
func (c *Context) HCLContextWithIteration(countIndex *int, eachKey, eachValue *cty.Value) *hcl.EvalContext {
	base := c.HCLContext()
	vars := maps.Clone(base.Variables)
	if countIndex != nil {
		vars["count"] = cty.ObjectVal(map[string]cty.Value{
			"index": cty.NumberIntVal(int64(*countIndex)),
		})
	}
	if eachKey != nil && eachValue != nil {
		vars["each"] = cty.ObjectVal(map[string]cty.Value{
			"key":   *eachKey,
			"value": *eachValue,
		})
	}
	return &hcl.EvalContext{
		Variables: vars,
		Functions: base.Functions,
	}
}

// Clone creates a copy of the context for isolated evaluation.
func (c *Context) Clone() *Context {
	c.mu.RLock()
	defer c.mu.RUnlock()

	clonedModuleOutputs := make(map[string]map[string]cty.Value, len(c.moduleOutputs))
	for k, v := range c.moduleOutputs {
		clonedModuleOutputs[k] = maps.Clone(v)
	}

	clonedRanged := make(map[string][]rangedInstance, len(c.rangedResources))
	for k, v := range c.rangedResources {
		clonedRanged[k] = append([]rangedInstance(nil), v...)
	}

	clone := &Context{
		mu:                new(sync.RWMutex),
		rootModuleDir:     c.rootModuleDir,
		providerFunctions: c.providerFunctions,
		variables:         maps.Clone(c.variables),
		locals:            maps.Clone(c.locals),
		resources:         maps.Clone(c.resources),
		rangedResources:   clonedRanged,
		dataSources:       maps.Clone(c.dataSources),
		modules:           maps.Clone(c.modules),
		moduleOutputs:     clonedModuleOutputs,
		calls:             maps.Clone(c.calls),
		path:              c.path,
		pulumi:            c.pulumi,
	}

	if c.count != nil {
		clone.count = &CountContext{Index: c.count.Index}
	}
	if c.each != nil {
		clone.each = &EachContext{Key: c.each.Key, Value: c.each.Value}
	}

	return clone
}
