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

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/runtime"
)

// registerRemovedProvisionerHooks registers the destroy-time provisioners
// declared in removed blocks. The engine destroys a resource absent from the
// program on its own; what it cannot do alone is resolve the BeforeDelete
// hook names recorded in the resource's state — every recorded name must be
// registered in this run's hook registry or the delete fails. Each
// removed-block provisioner is therefore registered under the name the
// resource bound while it was still declared:
// "<type>.<name>:provisioner:<i>" (see bindProvisionerHooks).
//
// ponytail: names are matched by position — provisioner i in the removed
// block must have been provisioner i on the resource, which holds when the
// resource's provisioners were all destroy-time and moved over in order. A
// resource that mixed create and destroy provisioners records shifted
// indexes and needs state-aware naming to line up; its delete fails loudly
// with "hook not registered". Instance-keyed names (count/for_each) are
// similarly out of reach.
func (e *Engine) registerRemovedProvisionerHooks(ctx context.Context) error {
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
		mapping := e.resolver.ResourceBodyMapping(ctx, typ)
		hclSnapshot := e.evaluator.Context().HCLContext()
		dryRun := e.dryRun

		for i, prov := range rem.Provisioners {
			prov, index := prov, i+1
			spec := &runtime.Spec{
				Type:   prov.Type,
				Config: prov.Config,
			}
			if prov.Connection != nil {
				spec.Conn = prov.Connection.Config
			}
			onFailureContinue := prov.OnFailure == "continue"
			hookName := fmt.Sprintf("%s.%s:provisioner:%d", typ, name, i)
			errLabel := fmt.Sprintf("removed %s.%s", typ, name)

			callback := func(hookCtx context.Context, args *ResourceHookArgs) error {
				outputs := args.OldOutputs
				// terraform_data's output mirrors input
				// (lowerTerraformDataOutputs); its triggers_replace echo needs
				// the registration-time trigger tuple, which a removed
				// resource no longer has, so a reference to it fails to
				// evaluate rather than reporting a fabricated value.
				if typ == packages.TerraformDataType {
					outputs = outputs.Set("output", outputs.Get("input"))
				}
				selfCtx, err := selfBoundEvalCtx(hclSnapshot, outputs, args.ID, args.URN, typ, resSchema, mapping, dryRun)
				if err != nil {
					return fmt.Errorf("provisioner %d for %s: %w", index, errLabel, err)
				}
				if runErr := runtime.Run(hookCtx, spec, selfCtx); runErr != nil {
					if onFailureContinue {
						return nil
					}
					return fmt.Errorf("provisioner %d (%s) for %s: %w",
						index, prov.Type, errLabel, runErr)
				}
				return nil
			}

			if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
				OnDryRun: false, // TF doesn't run provisioners during plan.
			}); err != nil {
				return fmt.Errorf("registering removed-block provisioner hook: %w", err)
			}
		}
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
