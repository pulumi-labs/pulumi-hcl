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
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/graph"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi-labs/pulumi-hcl/pkg/provisioner/runtime"
)

// bindProvisionerHooks binds AfterCreate (when=create) or BeforeDelete
// (when=destroy). TF does not re-run provisioners on update — no AfterUpdate
// binding. The HCL eval context is snapshotted now since the callback fires
// asynchronously, after processing has moved past this resource.
//
// Create-time provisioners keep per-instance hook names: their names are read
// back only from the state this same run writes. Destroy-time provisioners
// bind the constant [destroyProvisionerHook] instead — the name is read from
// *old* state on a later run, where this instance (or its whole block) may be
// gone from the program, so any per-instance name would be unregistered there
// and brick the delete. The instance's runner is keyed by its URN, computed
// before registration because pulumi-hcl defaults to delete-before-replace: a
// replacement's delete hook fires while RegisterResource is still blocked.
func (e *Engine) bindProvisionerHooks(
	ctx context.Context,
	res *ast.Resource,
	resSchema *schema.Resource,
	mapping *bridge.BodyMapping,
	instance *graph.ExpandedResource,
	evalCtx *eval.Context,
	opts *ResourceOptions,
	resourceName string,
) error {
	if len(res.Provisioners) == 0 {
		return nil
	}
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	dryRun := e.dryRun

	// runOne runs provisioner i (1-based index for messages) with self bound
	// to the given outputs.
	runOne := func(hookCtx context.Context, prov *ast.Provisioner, index int, outputs property.Map, id, urn string) error {
		spec := &runtime.Spec{
			Type:   prov.Type,
			Config: prov.Config,
			Conn:   effectiveConnectionBody(prov, res),
		}
		// Hooks receive raw engine outputs, so terraform_data's surface is
		// adapted the same way as on the registration path. This holds for
		// old outputs too: their {type, value} wrappers carry the
		// state-stored types, so a destroy-time self keeps them.
		outputs = lowerTerraformDataOutputs(res.Type, outputs, opts)
		selfCtx, err := selfBoundEvalCtx(hclSnapshot, outputs, id, urn, res.Type, resSchema, mapping, dryRun)
		if err != nil {
			return fmt.Errorf("provisioner %d for %s: %w", index, instance.Key, err)
		}
		if runErr := runtime.Run(hookCtx, spec, selfCtx); runErr != nil {
			if prov.OnFailure == "continue" {
				return nil
			}
			return fmt.Errorf("provisioner %d (%s) for %s: %w",
				index, prov.Type, instance.Key, runErr)
		}
		return nil
	}

	destroy := false
	for i, prov := range res.Provisioners {
		prov, index := prov, i+1
		if prov.When == "destroy" {
			destroy = true
			continue
		}
		hookName := fmt.Sprintf("%s.%s:provisioner:%d", res.Type, resourceName, i)
		callback := func(hookCtx context.Context, args *ResourceHookArgs) error {
			return runOne(hookCtx, prov, index, args.NewOutputs, args.ID, args.URN)
		}
		if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
			OnDryRun: false, // TF doesn't run provisioners during plan.
		}); err != nil {
			return fmt.Errorf("registering provisioner hook: %w", err)
		}
		opts.Hooks.AfterCreate = append(opts.Hooks.AfterCreate, hookName)
	}
	if !destroy {
		return nil
	}

	instanceURN, _ := e.resmon.ResolveURN(opts.Parent, resSchema.Token, resourceName)
	destroyDispatch.putInstance(e.deploymentKey, string(instanceURN),
		func(hookCtx context.Context, args *ResourceHookArgs) error {
			for i, prov := range res.Provisioners {
				if prov.When != "destroy" {
					continue
				}
				if err := runOne(hookCtx, prov, i+1, args.OldOutputs, args.ID, args.URN); err != nil {
					return err
				}
			}
			return nil
		})
	opts.Hooks.BeforeDelete = append(opts.Hooks.BeforeDelete, destroyProvisionerHook)
	e.registerLegacyProvisionerNames(ctx, res, resourceName, runOne)
	return nil
}

// registerLegacyProvisionerNames registers — without binding, so they never
// re-enter state — the per-instance destroy hook names earlier releases
// recorded in state, so a state written by one of them still deletes. Names
// are registered past the current provisioner count: the old state's list is
// as long as the *previous* config's count, and an unregistered name
// hard-errors while an unbound one costs nothing.
func (e *Engine) registerLegacyProvisionerNames(
	ctx context.Context, res *ast.Resource, resourceName string,
	runOne func(context.Context, *ast.Provisioner, int, property.Map, string, string) error,
) {
	legacyCap := len(res.Provisioners) + 4
	for i := range legacyCap {
		// A create-time provisioner at this index already holds the name (the
		// AfterCreate binding registered it); a legacy destroy name colliding
		// with it can only mean the provisioner list changed kind at this
		// index in the upgrade apply, which the migration accepts.
		if i < len(res.Provisioners) && res.Provisioners[i].When != "destroy" {
			continue
		}
		hookName := fmt.Sprintf("%s.%s:provisioner:%d", res.Type, resourceName, i)
		callback := func(hookCtx context.Context, args *ResourceHookArgs) error {
			if i >= len(res.Provisioners) || res.Provisioners[i].When != "destroy" {
				return nil
			}
			return runOne(hookCtx, res.Provisioners[i], i+1, args.OldOutputs, args.ID, args.URN)
		}
		if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
			OnDryRun: false, // TF doesn't run provisioners during plan.
		}); err != nil {
			_ = e.resmon.LogWarning(ctx, fmt.Sprintf("registering legacy provisioner hook %q: %v", hookName, err))
		}
	}
}

// effectiveConnectionBody: provisioner-level overrides resource-level.
func effectiveConnectionBody(prov *ast.Provisioner, res *ast.Resource) hcl.Body {
	if prov.Connection != nil {
		return prov.Connection.Config
	}
	if res.Connection != nil {
		return res.Connection.Config
	}
	return nil
}

// selfBoundEvalCtx binds `self` to resource outputs + the synthetic id/urn
// the engine injects elsewhere (see registerResourceInstanceInContext).
func selfBoundEvalCtx(
	parent *hcl.EvalContext, outputs property.Map, id, urn, tfType string,
	resSchema *schema.Resource, mapping *bridge.BodyMapping, dryRun bool,
) (*hcl.EvalContext, error) {
	if outputs.Len() == 0 && id == "" && urn == "" {
		return parent, nil
	}
	outputObj, err := transform.ResourceOutputToCty(outputs, resSchema, mapping, dryRun)
	if err != nil {
		return nil, fmt.Errorf("converting outputs: %w", err)
	}
	if err := unwrapTerraformDataOutputs(tfType, outputObj, outputs); err != nil {
		return nil, fmt.Errorf("converting outputs: %w", err)
	}
	if id != "" {
		outputObj["id"] = cty.StringVal(id)
	}
	if urn != "" {
		outputObj["urn"] = cty.StringVal(urn)
	}
	child := parent.NewChild()
	child.Variables = map[string]cty.Value{"self": cty.ObjectVal(outputObj)}
	return child, nil
}
