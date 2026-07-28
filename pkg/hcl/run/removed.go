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

package run

import (
	"context"
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
)

// recordRemovedBlockEntries records a destroy-dispatcher entry for each
// removed block that carries provisioners, from the graph's merged list:
// every block in the module tree, child-declared addresses rewritten to be
// root-relative. The engine destroys a resource absent from the program on
// its own and invokes the constant [destroyProvisionerHook] recorded in its
// state; the dispatcher matches the orphan to the removed block by config
// address (see [DestroyDispatcher]) and runs the block's provisioners,
// instance keys included. A resource whose state never recorded the hook —
// it had no destroy-time provisioners when last registered — deletes without
// running the removed block's provisioners.
func (e *Engine) recordRemovedBlockEntries(ctx context.Context) error {
	if e.resmon == nil {
		return nil
	}
	for _, rem := range e.graph.Removed() {
		// A destroy = false block already carries a parse-time error
		// diagnostic; its provisioners must not run.
		if !rem.Destroy || len(rem.Provisioners) == 0 {
			continue
		}
		module, typ, name, err := removedResourceAddr(rem.From)
		if err != nil {
			return err
		}
		resSchema, err := e.resolver.ResolveResource(ctx, typ)
		if err != nil {
			return fmt.Errorf("removed block for %s.%s: resolving resource type: %w", typ, name, err)
		}
		res := &ast.Resource{
			Type:         typ,
			Name:         name,
			Provisioners: rem.Provisioners,
			DeclRange:    rem.DeclRange,
		}
		e.recordRemovedEntry(ctx, res, resSchema, module)
	}
	return nil
}

// recordRemovedEntry records the dispatch entry for a removed block's
// resource address. Unlike a live block's entry it carries no module instance
// scope — a module-qualified orphan evaluates its provisioners in TF's
// destroy scope (self, count.index, each.key, path.*, terraform.*), which is
// also all TF allows a removed block's provisioners to reference.
func (e *Engine) recordRemovedEntry(
	ctx context.Context, res *ast.Resource, resSchema *schema.Resource, module modulepath.Path,
) {
	probeURN, probeName := e.resmon.ResolveURN(e.stackURN, resSchema.Token, "probe")
	entry := &blockEntry{
		resmon:       e.resmon,
		dryRun:       e.dryRun,
		res:          res,
		resSchema:    resSchema,
		mapping:      e.resolver.ResourceBodyMapping(ctx, res.Type),
		token:        resSchema.Token,
		prefix:       strings.TrimSuffix(probeName, "probe"),
		evalCtx:      e.evaluator.Context(),
		config:       modulepath.NewAddress(module, modulepath.NewStep(res.Name)),
		moduleTarget: !module.IsRoot(),
	}
	_, entry.parentChain = urnTypes(string(probeURN))
	e.dispatcher.putBlock(entry)
}

// removedResourceAddr splits a removed block's root-relative "from" traversal
// into the enclosing module config path and the resource type and name. The
// parser has already rejected the address shapes that cannot carry
// provisioners (whole modules, instance keys, data sources).
func removedResourceAddr(from hcl.Traversal) (module modulepath.Path, typ, name string, err error) {
	var names []string
	for _, step := range from {
		switch s := step.(type) {
		case hcl.TraverseRoot:
			names = append(names, s.Name)
		case hcl.TraverseAttr:
			names = append(names, s.Name)
		}
	}
	module = modulepath.Root()
	for len(names) >= 2 && names[0] == "module" {
		module = module.Append(modulepath.NewStep(names[1]))
		names = names[2:]
	}
	if len(names) != 2 {
		return module, "", "", fmt.Errorf("removed block: expected a resource address (type.name)")
	}
	return module, names[0], names[1], nil
}
