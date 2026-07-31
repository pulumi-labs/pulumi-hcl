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

package converter

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"iter"
	"maps"
	"os"
	"slices"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi-hcl/pkg/util/encryption"
	"github.com/pulumi/pulumi-hcl/vendored/addrs"
	"github.com/pulumi/pulumi-hcl/vendored/statefile"
	"github.com/pulumi/pulumi-hcl/vendored/states"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
)

// ConvertState reads a Terraform/OpenTofu state file and emits a
// ResourceImport per managed root-module resource instance, resolving TF types
// to Pulumi tokens through the engine-provided mapper, so
// `pulumi import --from hcl` can pull TF state into a Pulumi HCL project.
func (*hclConverter) ConvertState(
	ctx context.Context, req *plugin.ConvertStateRequest,
) (*plugin.ConvertStateResponse, error) {
	if req.MapperTarget == "" {
		return nil, errors.New("ConvertState: missing mapper target")
	}
	if len(req.Args) != 1 {
		return nil, fmt.Errorf(
			"ConvertState: expected exactly one argument (the state file path), got %d", len(req.Args))
	}
	statePath := req.Args[0]

	mapperClient, err := convert.NewMapperClient(req.MapperTarget)
	if err != nil {
		return nil, fmt.Errorf("dial mapper at %s: %w", req.MapperTarget, err)
	}
	providerInfoSource := bridge.NewCache(bridge.NewMapperSource(mapperClient))

	state, err := readTFStateFile(statePath)
	if err != nil {
		return nil, err
	}

	return convertTFState(ctx, providerInfoSource, state), nil
}

// readTFStateFile parses a state file through the vendored OpenTofu parser,
// which sniffs the format version and upgrades pre-v4 states.
func readTFStateFile(statePath string) (*states.State, error) {
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("reading state file %q: %w", statePath, err)
	}
	f, err := statefile.Read(bytes.NewReader(data), encryption.StateEncryptionDisabled())
	if err != nil {
		return nil, fmt.Errorf("parsing state file %q: %w", statePath, err)
	}
	return f.State, nil
}

// convertTFState is the pure core of ConvertState: it turns a parsed state
// into ResourceImports given a resolved provider-info source. Anything that
// can't be imported is reported as a warning diagnostic rather than failing
// the whole conversion.
func convertTFState(
	ctx context.Context,
	providerInfoSource bridge.ProviderInfoSource,
	state *states.State,
) *plugin.ConvertStateResponse {
	var (
		resources   []plugin.ResourceImport
		diagnostics hcl.Diagnostics
	)
	// Import obstacles are warnings so the importable remainder still lands:
	// error diagnostics would make the CLI abort the import entirely.
	warn := func(summary, detail string) {
		diagnostics = append(diagnostics, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  summary,
			Detail:   detail,
		})
	}

	for res := range managedResources(state, warn) {
		provider := res.ProviderConfig.Provider.Type
		tfType := res.Addr.Resource.Type
		info, err := providerInfoSource.GetProviderInfo(ctx, provider, nil)
		if err != nil || info == nil {
			warn("Failed to resolve provider", fmt.Sprintf(
				"could not resolve bridge mapping for provider %q: %v", provider, err))
			continue
		}
		resInfo, ok := info.Resources[tfType]
		if !ok || resInfo == nil || resInfo.Tok == "" {
			warn("Failed to resolve resource type", fmt.Sprintf(
				"provider %q has no mapping for TF type %q", provider, tfType))
			continue
		}
		token := resInfo.Tok.String()

		for _, key := range sortedInstanceKeys(res) {
			current := res.Instances[key].Current
			if current == nil {
				continue
			}
			id, ok := importID(current)
			if !ok {
				warn("Skipped resource without id", fmt.Sprintf(
					"an instance of %s has no string `id` attribute to import by", res.Addr))
				continue
			}
			outs, ins, err := translateInstanceValues(ctx, info, tfType, current)
			if err != nil {
				warn("Importing without values", fmt.Sprintf(
					"an instance of %s imports by id only: %v", res.Addr, err))
			}
			resources = append(resources, plugin.ResourceImport{
				Type:    token,
				Name:    resourceName(res.Addr.Resource.Name, key),
				ID:      id,
				Inputs:  ins,
				Outputs: outs,
			})
		}
	}

	return &plugin.ConvertStateResponse{Resources: resources, Diagnostics: diagnostics}
}

// managedResources yields the root module's managed resources in address
// order. Module-nested resources are skipped with a warning: core can import
// components (pulumi/pulumi#15199), but mapping module instances to the
// component names the HCL runtime registers is not built yet
// (pulumi/pulumi-hcl#167).
func managedResources(state *states.State, warn func(summary, detail string)) iter.Seq[*states.Resource] {
	return func(yield func(*states.Resource) bool) {
		for _, modKey := range slices.Sorted(maps.Keys(state.Modules)) {
			mod := state.Modules[modKey]
			for _, resKey := range slices.Sorted(maps.Keys(mod.Resources)) {
				res := mod.Resources[resKey]
				if res.Addr.Resource.Mode != addrs.ManagedResourceMode {
					continue
				}
				if !mod.Addr.IsRoot() {
					warn("Skipped module resource", fmt.Sprintf(
						"resource %q is nested in %q; module import is not yet supported",
						res.Addr.Resource.Name, mod.Addr.String()))
					continue
				}
				if !yield(res) {
					return
				}
			}
		}
	}
}

// sortedInstanceKeys orders a resource's instances deterministically:
// singletons first, count instances numerically, for_each keys lexically.
func sortedInstanceKeys(res *states.Resource) []addrs.InstanceKey {
	return slices.SortedFunc(maps.Keys(res.Instances), func(a, b addrs.InstanceKey) int {
		switch {
		case addrs.InstanceKeyLess(a, b):
			return -1
		case addrs.InstanceKeyLess(b, a):
			return 1
		default:
			return 0
		}
	})
}

// resourceName reproduces the HCL runtime's buildResourceName so imported
// resources land on the URNs the program will register: the modulepath step
// naming for expanded instances, and deliberately no sanitisation, unlike
// pulumi-converter-terraform — the runtime registers raw labels, so
// sanitising here would diff.
func resourceName(name string, key addrs.InstanceKey) string {
	switch k := key.(type) {
	case addrs.IntKey:
		return modulepath.NewIndexedStep(name, int(k)).LogicalName()
	case addrs.StringKey:
		return modulepath.NewKeyedStep(name, string(k)).LogicalName()
	default:
		return name
	}
}

// translateInstanceValues turns an instance's raw TF attributes into the
// Pulumi property maps an outputs-supplied import needs, using the bridge's
// own translation over the mapper-provided schema-only shim — so Outputs are
// exactly what the bridged provider would have written (renames, MaxItemsOne
// flattening, schema-sensitive fields as secrets) and Inputs are derived the
// way the bridge's import path derives them. With Outputs supplied the engine
// skips the provider's Read entirely.
func translateInstanceValues(
	ctx context.Context,
	info *tfbridge.ProviderInfo,
	tfType string,
	current *states.ResourceInstanceObjectSrc,
) (outs, ins resource.PropertyMap, err error) {
	if current.AttrsJSON == nil {
		return nil, nil, errors.New("pre-0.12 flatmap attributes are not translatable")
	}
	if info.P == nil {
		return nil, nil, errors.New("the mapping carries no provider schema")
	}
	shimRes, ok := info.P.ResourcesMap().GetOk(tfType)
	if !ok {
		return nil, nil, fmt.Errorf("provider schema has no resource %q", tfType)
	}
	// The attributes were written at the instance's schema version; without a
	// live provider there is no UpgradeResourceState, so drifted values
	// project as-is.
	if int(current.SchemaVersion) != shimRes.SchemaVersion() {
		return nil, nil, fmt.Errorf(
			"state attributes have schema version %d but the provider maps version %d",
			current.SchemaVersion, shimRes.SchemaVersion())
	}
	var attrs map[string]any
	if err := json.Unmarshal(current.AttrsJSON, &attrs); err != nil {
		return nil, nil, fmt.Errorf("decoding attributes: %w", err)
	}

	var fields map[string]*tfbridge.SchemaInfo
	if resInfo := info.Resources[tfType]; resInfo != nil {
		fields = resInfo.Fields
	}
	outs = tfbridge.MakeTerraformOutputs(ctx, jsonSetChecker{}, attrs, shimRes.Schema(), fields, nil, true)
	applySensitivePaths(outs, current.AttrSensitivePaths, bridge.ResourceBodyMapping(info, tfType))
	ins, err = tfbridge.ExtractInputsFromOutputs(nil, outs, shimRes.Schema(), fields, false)
	if err != nil {
		return nil, nil, fmt.Errorf("deriving inputs: %w", err)
	}
	return outs, ins, nil
}

// jsonSetChecker satisfies tfbridge's SetChecker for JSON-decoded attribute
// values, which never contain live *schema.Set instances — set-typed
// attributes arrive as plain arrays. (The mapper-delivered schema-only shim
// panics on IsSet.)
type jsonSetChecker struct{}

func (jsonSetChecker) IsSet(context.Context, any) ([]any, bool) { return nil, false }

// applySensitivePaths marks the values at the state's dynamically-sensitive
// paths as secrets. Schema-declared sensitivity is already handled during
// translation; these are the residual marks (e.g. TF's sensitive() function).
// A path that does not translate or resolve is skipped — better an unmarked
// value than a failed import.
func applySensitivePaths(outs resource.PropertyMap, paths []cty.PathValueMarks, mapping *bridge.BodyMapping) {
	if len(paths) == 0 {
		return
	}
	v := resource.FromResourcePropertyValue(resource.NewObjectProperty(outs))
	for _, pvm := range paths {
		pp, err := transform.TranslateAttrPath(pvm.Path, mapping, nil)
		if err != nil {
			continue
		}
		if altered, err := pp.Alter(v, func(e property.Value) property.Value {
			return e.WithSecret(true)
		}); err == nil {
			v = altered
		}
	}
	marked := resource.ToResourcePropertyValue(v).ObjectValue()
	clear(outs)
	maps.Copy(outs, marked)
}

// importID extracts the `id` attribute verbatim.
//
// TODO(pulumi/pulumi-hcl#167): resources whose importer needs a
// composite or derived import string (e.g. aws_iam_role_policy_attachment's
// role/policy-arn) import under the wrong ID; deriving the import string
// needs per-resource knowledge the schema does not carry.
func importID(src *states.ResourceInstanceObjectSrc) (string, bool) {
	if src.AttrsJSON == nil {
		// Pre-0.12 states carry flatmap attributes instead.
		id := src.AttrsFlat["id"]
		return id, id != ""
	}
	var attrs struct {
		ID string `json:"id"`
	}
	if err := json.Unmarshal(src.AttrsJSON, &attrs); err != nil || attrs.ID == "" {
		return "", false
	}
	return attrs.ID, true
}
