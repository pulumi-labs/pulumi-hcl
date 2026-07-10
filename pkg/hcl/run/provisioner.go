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
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
func (e *Engine) bindProvisionerHooks(
	ctx context.Context,
	res *ast.Resource,
	resSchema *schema.Resource,
	mapping *bridge.BodyMapping,
	instance *graph.ExpandedResource,
	evalCtx *eval.Context,
	opts *ResourceOptions,
) error {
	if len(res.Provisioners) == 0 {
		return nil
	}
	if opts.Hooks == nil {
		opts.Hooks = &ResourceHookBinding{}
	}
	hclSnapshot := evalCtx.HCLContext()
	dryRun := e.dryRun

	for i, prov := range res.Provisioners {
		prov, index := prov, i+1
		spec := &runtime.Spec{
			Type:   prov.Type,
			Config: prov.Config,
			Conn:   effectiveConnectionBody(prov, res),
		}
		when := prov.When
		if when == "" {
			when = "create"
		}
		onFailureContinue := prov.OnFailure == "continue"
		useOldOutputs := when == "destroy"
		hookName := fmt.Sprintf("%s:provisioner:%d", instance.Key, i)

		callback := func(hookCtx context.Context, args *ResourceHookArgs) error {
			outputs := args.NewOutputs
			if useOldOutputs {
				outputs = args.OldOutputs
			}
			selfCtx, err := selfBoundEvalCtx(hclSnapshot, outputs, args.ID, args.URN, resSchema, mapping, dryRun)
			if err != nil {
				return fmt.Errorf("provisioner %d for %s: %w", index, instance.Key, err)
			}
			if runErr := runtime.Run(hookCtx, spec, selfCtx); runErr != nil {
				if onFailureContinue {
					return nil
				}
				return fmt.Errorf("provisioner %d (%s) for %s: %w",
					index, prov.Type, instance.Key, runErr)
			}
			return nil
		}

		if err := e.resmon.RegisterResourceHook(ctx, hookName, callback, ResourceHookOptions{
			OnDryRun: false, // TF doesn't run provisioners during plan.
		}); err != nil {
			return fmt.Errorf("registering provisioner hook: %w", err)
		}

		if useOldOutputs {
			opts.Hooks.BeforeDelete = append(opts.Hooks.BeforeDelete, hookName)
		} else {
			opts.Hooks.AfterCreate = append(opts.Hooks.AfterCreate, hookName)
		}
	}
	return nil
}

// provisionerDependencyKeys collects the resource/data/module references
// made inside a resource's connection block and its create-time provisioner
// bodies, so they participate in dependency ordering like regular attribute
// references. Destroy-time provisioners may only reference self, count and
// each, so they contribute nothing.
func provisionerDependencyKeys(res *ast.Resource) []string {
	var bodies []hcl.Body
	if res.Connection != nil {
		bodies = append(bodies, res.Connection.Config)
	}
	for _, prov := range res.Provisioners {
		if prov.When == "destroy" {
			continue
		}
		if prov.Connection != nil {
			bodies = append(bodies, prov.Connection.Config)
		}
		bodies = append(bodies, prov.Config)
	}

	var keys []string
	seen := make(map[string]bool)
	var walk func(body hcl.Body)
	walk = func(body hcl.Body) {
		syntaxBody, ok := body.(*hclsyntax.Body)
		if !ok {
			return
		}
		for _, attr := range syntaxBody.Attributes {
			for _, dep := range eval.ExtractDependencies(attr.Expr) {
				if !seen[dep] {
					seen[dep] = true
					keys = append(keys, dep)
				}
			}
		}
		for _, block := range syntaxBody.Blocks {
			walk(block.Body)
		}
	}
	for _, body := range bodies {
		if body != nil {
			walk(body)
		}
	}
	slices.Sort(keys)
	return keys
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
	parent *hcl.EvalContext, outputs property.Map, id, urn string,
	resSchema *schema.Resource, mapping *bridge.BodyMapping, dryRun bool,
) (*hcl.EvalContext, error) {
	if outputs.Len() == 0 && id == "" && urn == "" {
		return parent, nil
	}
	outputObj, err := transform.ResourceOutputToCty(outputs, resSchema, mapping, dryRun)
	if err != nil {
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
