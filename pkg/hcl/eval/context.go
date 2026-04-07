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
	"maps"
	"sort"
	"strings"
	"sync"

	"github.com/hashicorp/hcl/v2"
	"github.com/zclconf/go-cty/cty"
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
	// mu protects concurrent access to maps
	mu sync.RWMutex

	// baseDir is the base directory for file operations
	baseDir string

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

	// providers contains provider references (provider.name)
	providers map[string]cty.Value

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

	// self contains the current resource for self references
	self cty.Value
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

// NewContext creates a new evaluation context.
func NewContext(baseDir, rootDir, stack, project, organization string) *Context {
	return &Context{
		baseDir: baseDir,

		variables:       make(map[string]cty.Value),
		locals:          make(map[string]cty.Value),
		resources:       make(map[string]cty.Value),
		rangedResources: make(map[string][]rangedInstance),
		dataSources:     make(map[string]cty.Value),
		modules:       make(map[string]cty.Value),
		moduleOutputs: make(map[string]map[string]cty.Value),
		providers:   make(map[string]cty.Value),
		calls:       make(map[string]cty.Value),
		path: PathContext{
			Module: baseDir,
			Root:   rootDir,
			Cwd:    baseDir,
		},
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

// SetResource sets a resource's output values.
// The key should be "type.name" (e.g., "aws_instance.web").
func (c *Context) SetResource(key string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.resources[key] = value
}

// SetCountResource stores a resource instance from count expansion.
// baseKey is the resource key without the index suffix (e.g., "type.name").
func (c *Context) SetCountResource(baseKey string, index int, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rangedResources[baseKey] = append(c.rangedResources[baseKey], rangedInstance{
		value: value, index: index, isCount: true,
	})
}

// SetEachResource stores a resource instance from for_each expansion.
// baseKey is the resource key without the each key suffix (e.g., "type.name").
func (c *Context) SetEachResource(baseKey string, eachKey string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.rangedResources[baseKey] = append(c.rangedResources[baseKey], rangedInstance{
		value: value, eachKey: eachKey, isEach: true,
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

// SetProvider sets a provider reference.
func (c *Context) SetProvider(name string, value cty.Value) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.providers[name] = value
}

// SetCount sets the count context for count iteration.
func (c *Context) SetCount(index int) {
	c.count = &CountContext{Index: index}
}

// ClearCount clears the count context.
func (c *Context) ClearCount() {
	c.count = nil
}

// SetEach sets the each context for for_each iteration.
func (c *Context) SetEach(key, value cty.Value) {
	c.each = &EachContext{Key: key, Value: value}
}

// ClearEach clears the each context.
func (c *Context) ClearEach() {
	c.each = nil
}

// SetSelf sets the self reference (for provisioner expressions).
func (c *Context) SetSelf(value cty.Value) {
	c.self = value
}

// ClearSelf clears the self reference.
func (c *Context) ClearSelf() {
	c.self = cty.NilVal
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
			sort.Slice(instances, func(i, j int) bool {
				return instances[i].index < instances[j].index
			})
			vals := make([]cty.Value, len(instances))
			for i, inst := range instances {
				vals[i] = inst.value
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
	vars["pulumi"] = cty.ObjectVal(map[string]cty.Value{
		"stack":        cty.StringVal(c.pulumi.Stack),
		"project":      cty.StringVal(c.pulumi.Project),
		"organization": cty.StringVal(c.pulumi.Organization),
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

	// Add self if set (for provisioners)
	if c.self != cty.NilVal {
		vars["self"] = c.self
	}

	return &hcl.EvalContext{
		Variables: vars,
		Functions: Functions(c.baseDir),
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
		baseDir:         c.baseDir,
		variables:       maps.Clone(c.variables),
		locals:          maps.Clone(c.locals),
		resources:       maps.Clone(c.resources),
		rangedResources: clonedRanged,
		dataSources:     maps.Clone(c.dataSources),
		modules:       maps.Clone(c.modules),
		moduleOutputs: clonedModuleOutputs,
		providers:     maps.Clone(c.providers),
		calls:         maps.Clone(c.calls),
		path:          c.path,
		pulumi:        c.pulumi,
		self:          c.self,
	}

	if c.count != nil {
		clone.count = &CountContext{Index: c.count.Index}
	}
	if c.each != nil {
		clone.each = &EachContext{Key: c.each.Key, Value: c.each.Value}
	}

	return clone
}
