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
	"strings"

	"github.com/blang/semver"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/parser"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi-hcl/pkg/util/encryption"
	"github.com/pulumi/pulumi-hcl/vendored/addrs"
	"github.com/pulumi/pulumi-hcl/vendored/statefile"
	"github.com/pulumi/pulumi-hcl/vendored/states"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	ctyjson "github.com/zclconf/go-cty/cty/json"
)

// ConvertState reads a Terraform/OpenTofu state file and emits a
// ResourceImport per managed root-module resource instance, resolving TF types
// to Pulumi tokens through the engine-provided mapper, so
// `pulumi import --from hcl` can pull TF state into a Pulumi HCL project.
// Imports are parameterized (dynamic-bridge) when the project's terraform
// blocks say so, landing resources under the same `terraform-provider`
// packages the HCL runtime executes — unlike pulumi-converter-terraform's
// static-bridge imports, which would provoke a replace on the next preview.
func (*hclConverter) ConvertState(
	ctx context.Context, req *plugin.ConvertStateRequest,
) (*plugin.ConvertStateResponse, error) {
	if req.MapperTarget == "" {
		return nil, errors.New("ConvertState: missing mapper target")
	}
	if req.LoaderTarget == "" {
		return nil, errors.New("ConvertState: missing loader address")
	}
	if len(req.Args) < 1 || len(req.Args) > 2 {
		return nil, fmt.Errorf(
			"ConvertState: expected the state file path and optionally the project directory, got %d arguments",
			len(req.Args))
	}
	statePath := req.Args[0]
	// The CLI runs the converter with cwd = project root; in-process callers,
	// which cannot chdir, pass the directory explicitly.
	projectDir := "."
	if len(req.Args) == 2 {
		projectDir = req.Args[1]
	}
	// Dynamically bridged providers are recognized from the project's own
	// terraform blocks, so import works before `pulumi install` has run. The
	// sdks/ descriptors install writes carry resolved versions, so they take
	// precedence when present.
	descriptors := deriveParameterizationInfos(projectDir)
	fromSDKs, err := readParameterizationInfos(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading parameterization infos: %w", err)
	}
	if descriptors == nil {
		descriptors = fromSDKs
	} else {
		maps.Copy(descriptors, fromSDKs)
	}

	mapperClient, err := convert.NewMapperClient(req.MapperTarget)
	if err != nil {
		return nil, fmt.Errorf("dial mapper at %s: %w", req.MapperTarget, err)
	}
	providerInfoSource := bridge.NewCache(bridge.NewMapperSource(mapperClient))

	loaderClient, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("dial loader at %s: %w", req.LoaderTarget, err)
	}
	defer contract.IgnoreClose(loaderClient)
	loader := schema.NewCachedLoader(loaderClient)
	if len(descriptors) > 0 {
		loader = packages.NewParameterizationAwareLoader(loader, descriptors)
	}

	state, err := readTFStateFile(statePath)
	if err != nil {
		return nil, err
	}

	return convertTFState(ctx, providerInfoSource, loader, descriptors, state), nil
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

// deriveParameterizationInfos derives parameterization descriptors from the
// project's terraform blocks: required_providers entries that are neither
// builtin nor pulumi-sourced are dynamically bridged via terraform-provider.
// An exact version pin becomes the package version; a range constraint leaves
// the version for the engine to resolve, but still parameterizes.
func deriveParameterizationInfos(dir string) map[string]workspace.PackageDescriptor {
	config, diags := parser.NewParser().ParseDirectory(dir)
	if config == nil || diags.HasErrors() || config.Terraform == nil {
		return nil
	}
	out := map[string]workspace.PackageDescriptor{}
	for alias, req := range config.Terraform.RequiredProviders {
		if alias == "pulumi" || alias == "terraform" || req.IsPulumi() {
			continue
		}
		source := "hashicorp/" + alias
		var constraint string
		if req != nil {
			if req.Source != "" {
				source = req.Source
			}
			constraint = req.Version
		}
		var version semver.Version
		if v, err := semver.ParseTolerant(constraint); err == nil {
			version = v
		}
		value, err := json.Marshal(map[string]any{"remote": map[string]string{
			"url": source, "version": constraint,
		}})
		contract.AssertNoErrorf(err, "marshaling the terraform-provider parameterization")
		parts := strings.Split(source, "/")
		out[alias] = workspace.PackageDescriptor{
			PluginDescriptor: workspace.PluginDescriptor{
				Name: "terraform-provider",
				Kind: apitype.ResourcePlugin,
			},
			Parameterization: &workspace.Parameterization{
				Name:    parts[len(parts)-1],
				Version: version,
				Value:   value,
			},
		}
	}
	return out
}

// convertTFState is the pure core of ConvertState: it turns a parsed state
// into ResourceImports given a resolved provider-info source. Anything that
// can't be imported is reported as a warning diagnostic rather than failing
// the whole conversion.
func convertTFState(
	ctx context.Context,
	providerInfoSource bridge.ProviderInfoSource,
	loader schema.ReferenceLoader,
	descriptors map[string]workspace.PackageDescriptor,
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
		stash := tfType == packages.TerraformDataType
		token := packages.StashToken
		var info *tfbridge.ProviderInfo
		var desc *workspace.PackageDescriptor
		var parameterization *plugin.ResourceParameterization
		var version, pluginDownloadURL string
		if !stash {
			if d, ok := descriptors[provider]; ok {
				desc = &d
			}
			var err error
			info, err = providerInfoSource.GetProviderInfo(ctx, provider, desc)
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
			token = resInfo.Tok.String()
			parameterization, version = parameterizationFor(desc)
			if desc != nil {
				pluginDownloadURL = desc.PluginDownloadURL
			}
		}

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
			var outs, ins resource.PropertyMap
			var err error
			if stash {
				if outs, ins, err = stashValues(current); err != nil {
					warn("Skipped terraform_data resource", fmt.Sprintf(
						"an instance of %s cannot be translated: %v", res.Addr, err))
					continue
				}
			} else if outs, ins, err = translateInstanceValues(ctx, loader, info, token, tfType, id, current); err != nil {
				warn("Importing without values", fmt.Sprintf(
					"an instance of %s imports by id only: %v", res.Addr, err))
			}
			resources = append(resources, plugin.ResourceImport{
				Type:              token,
				Name:              resourceName(res.Addr.Resource.Name, key),
				ID:                id,
				Inputs:            ins,
				Outputs:           outs,
				Version:           version,
				PluginDownloadURL: pluginDownloadURL,
				Parameterization:  parameterization,
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

// parameterizationFor maps a package descriptor to the import-side
// parameterization: the resource's Type/Version describe the parameterized
// package (e.g. "aws@6.0.0"), while the Parameterization block describes the
// base plugin it is produced from (e.g. "terraform-provider").
func parameterizationFor(desc *workspace.PackageDescriptor) (*plugin.ResourceParameterization, string) {
	if desc == nil || desc.Parameterization == nil {
		return nil, ""
	}
	var baseVersion string
	if desc.Version != nil {
		baseVersion = desc.Version.String()
	}
	version := ""
	if v := desc.Parameterization.Version; !v.EQ(semver.Version{}) {
		version = v.String()
	}
	return &plugin.ResourceParameterization{
		PluginName:    desc.Name,
		PluginVersion: baseVersion,
		Value:         desc.Parameterization.Value,
	}, version
}

// translateInstanceValues converts an instance's raw TF attributes into
// Pulumi Outputs/Inputs through the loader-supplied Pulumi schema and the
// transform codec — the same pair the runtime uses for programs. With
// Outputs supplied the engine skips the provider's Read.
func translateInstanceValues(
	ctx context.Context,
	loader schema.ReferenceLoader,
	info *tfbridge.ProviderInfo,
	token, tfType, id string,
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
	// Without a live provider there is no UpgradeResourceState, so
	// version-drifted attributes cannot be translated.
	if int(current.SchemaVersion) != shimRes.SchemaVersion() {
		return nil, nil, fmt.Errorf(
			"state attributes have schema version %d but the provider maps version %d",
			current.SchemaVersion, shimRes.SchemaVersion())
	}
	res, err := resourceSchema(ctx, loader, token)
	if err != nil {
		return nil, nil, err
	}

	ty, err := ctyjson.ImpliedType(current.AttrsJSON)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding attributes: %w", err)
	}
	val, err := ctyjson.Unmarshal(current.AttrsJSON, ty)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding attributes: %w", err)
	}
	val = markSensitivePaths(val, current.AttrSensitivePaths)

	m, err := transform.CtyToResourceOutputs(val, res, bridge.ResourceBodyMapping(info, tfType))
	if err != nil {
		return nil, nil, fmt.Errorf("translating attributes: %w", err)
	}

	outs = resource.ToResourcePropertyValue(property.New(m)).ObjectValue()
	// Inputs are the outputs' input-property subset. Nested computed leaves
	// are not stripped; revisit if the round-trip check flags diffs.
	ins = make(resource.PropertyMap, len(res.InputProperties))
	for _, p := range res.InputProperties {
		if pv, has := outs[resource.PropertyKey(p.Name)]; has {
			ins[resource.PropertyKey(p.Name)] = pv
		}
	}
	outs["id"] = resource.NewStringProperty(id)
	return outs, ins, nil
}

// markSensitivePaths re-marks the state's dynamically-sensitive paths (e.g.
// TF's sensitive()) with the evaluator's mark; the transform codec turns marks
// into secrets. Schema-declared sensitivity is applied by the codec itself.
func markSensitivePaths(val cty.Value, paths []cty.PathValueMarks) cty.Value {
	if len(paths) == 0 {
		return val
	}
	pvms := make([]cty.PathValueMarks, len(paths))
	for i, p := range paths {
		pvms[i] = cty.PathValueMarks{Path: p.Path, Marks: cty.NewValueMarks(eval.SensitiveMark)}
	}
	return val.MarkWithPaths(pvms)
}

// terraformDataStateType is terraform_data's object type in state attributes;
// decoding against it lets ctyjson consume the dynamic attributes' type
// annotations natively.
var terraformDataStateType = cty.Object(map[string]cty.Type{
	"id":               cty.String,
	"input":            cty.DynamicPseudoType,
	"output":           cty.DynamicPseudoType,
	"triggers_replace": cty.DynamicPseudoType,
})

// stashValues translates a terraform_data instance's attributes into Stash's
// property surface — the resource the runtime lowers terraform_data onto,
// served by the engine's builtin provider. Inputs and Outputs reproduce what
// the runtime registers: {type, value} wrappers for non-null values, an
// explicit null input otherwise. triggers_replace is not emitted — Stash
// rejects it as an input and an import cannot carry the engine's replacement
// trigger; the trigger records on the first up, which the engine forgives
// without a replace. An untranslatable instance is skipped outright: with no
// plugin behind Stash, an id-only import could only fail.
func stashValues(current *states.ResourceInstanceObjectSrc) (outs, ins resource.PropertyMap, err error) {
	if current.AttrsJSON == nil {
		return nil, nil, errors.New("pre-0.12 flatmap attributes are not translatable")
	}
	val, err := ctyjson.Unmarshal(current.AttrsJSON, terraformDataStateType)
	if err != nil {
		return nil, nil, fmt.Errorf("decoding attributes: %w", err)
	}
	val = markSensitivePaths(val, current.AttrSensitivePaths)

	m, err := transform.CtyToResourceOutputs(val, packages.TerraformDataSchema(), nil)
	if err != nil {
		return nil, nil, fmt.Errorf("translating attributes: %w", err)
	}
	wrap := func(name string) resource.PropertyValue {
		v := m.Get(name)
		if v.IsNull() {
			return resource.NewNullProperty()
		}
		src, _ := val.GetAttr(name).Unmark()
		return resource.ToResourcePropertyValue(packages.WrapTerraformDataValue(src.Type(), v))
	}
	input := wrap("input")
	ins = resource.PropertyMap{"input": input}
	outs = resource.PropertyMap{"input": input, "output": wrap("output")}
	return outs, ins, nil
}

// resourceSchema loads token's resource schema through the engine's loader.
func resourceSchema(
	ctx context.Context, loader schema.ReferenceLoader, token string,
) (*schema.Resource, error) {
	pkgName := string(tokens.Type(token).Module().Package())
	ref, err := loader.LoadPackageReferenceV2(ctx, &schema.PackageDescriptor{Name: pkgName})
	if err != nil {
		return nil, fmt.Errorf("loading schema for package %q: %w", pkgName, err)
	}
	res, ok, err := ref.Resources().Get(token)
	if err != nil || !ok {
		return nil, fmt.Errorf("package %q has no resource %q: %v", pkgName, token, err)
	}
	return res, nil
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
