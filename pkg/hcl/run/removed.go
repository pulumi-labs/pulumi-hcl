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

	"github.com/hashicorp/hcl/v2"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
)

// recordRemovedBlockEntries records a destroy-dispatcher entry for each
// removed block that carries provisioners. The engine destroys a resource
// absent from the program on its own and invokes the constant
// [destroyProvisionerHook] recorded in its state; the dispatcher matches the
// orphan to the removed block by config address (see [DestroyDispatcher]) and
// runs the block's provisioners, instance keys included. A resource whose
// state never recorded the hook — it had no destroy-time provisioners when
// last registered — deletes without running the removed block's provisioners.
func (e *Engine) recordRemovedBlockEntries(ctx context.Context) error {
	if e.resmon == nil {
		return nil
	}
	for _, rem := range e.config.Removed {
		// A destroy = false block already carries a parse-time error
		// diagnostic; its provisioners must not run.
		if !rem.Destroy || len(rem.Provisioners) == 0 {
			continue
		}
		typ, name, err := removedResourceAddr(rem.From)
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
		e.recordBlockEntry(ctx, res, resSchema, e.evaluator.Context(), e.stackURN, nil)
	}
	return nil
}

// removedResourceAddr splits a removed block's "from" traversal into resource
// type and name. The parser has already rejected the address shapes that
// cannot carry provisioners (modules, instance keys, data sources).
func removedResourceAddr(from hcl.Traversal) (typ, name string, err error) {
	if len(from) == 2 {
		root, rootOK := from[0].(hcl.TraverseRoot)
		attr, attrOK := from[1].(hcl.TraverseAttr)
		if rootOK && attrOK {
			return root.Name, attr.Name, nil
		}
	}
	return "", "", fmt.Errorf("removed block: expected a root resource address (type.name)")
}
