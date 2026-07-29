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
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi-labs/pulumi-hcl/pkg/util/encryption"
	"github.com/pulumi-labs/pulumi-hcl/vendored/addrs"
	"github.com/pulumi-labs/pulumi-hcl/vendored/statefile"
	"github.com/pulumi-labs/pulumi-hcl/vendored/states"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// ConvertState reads a Terraform/OpenTofu state file and emits parameterized
// (dynamic-bridge) ResourceImports so `pulumi import --from hcl` lands TF state
// under the same `terraform-provider` packages the HCL runtime executes. This
// is what distinguishes it from pulumi-converter-terraform, which emits
// static-bridge imports that would provoke a replace on the next preview.
func (*hclConverter) ConvertState(
	ctx context.Context, req *plugin.ConvertStateRequest,
) (*plugin.ConvertStateResponse, error) {
	if req.MapperTarget == "" {
		return nil, errors.New("ConvertState: missing mapper target")
	}
	if len(req.Args) < 1 || len(req.Args) > 2 {
		return nil, fmt.Errorf(
			"ConvertState: expected the state file path and optionally the project directory, got %d arguments",
			len(req.Args))
	}
	statePath := req.Args[0]
	// The project directory holds the sdks/ descriptors. It defaults to the
	// working directory, which is the project root when the CLI runs the
	// converter; in-process callers pass it explicitly.
	projectDir := "."
	if len(req.Args) == 2 {
		projectDir = req.Args[1]
	}

	mapperClient, err := convert.NewMapperClient(req.MapperTarget)
	if err != nil {
		return nil, fmt.Errorf("dial mapper at %s: %w", req.MapperTarget, err)
	}
	providerInfoSource := bridge.NewCache(bridge.NewMapperSource(mapperClient))

	// Parameterization descriptors are written by `pulumi install` into sdks/.
	// `pulumi import` runs with cwd = project root, so they live at ./sdks/*.
	descriptors, err := readParameterizationInfos(projectDir)
	if err != nil {
		return nil, fmt.Errorf("reading parameterization infos: %w", err)
	}

	state, err := readTFStateFile(statePath)
	if err != nil {
		return nil, err
	}

	return convertTFState(ctx, providerInfoSource, descriptors, state), nil
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
// into ResourceImports given a resolved provider-info source and the on-disk
// parameterization descriptors. Anything that can't be imported is reported as
// a warning diagnostic rather than failing the whole conversion.
func convertTFState(
	ctx context.Context,
	providerInfoSource bridge.ProviderInfoSource,
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
		// A missing descriptor is not an error: only dynamically bridged
		// providers have one, and providers without it import as plain
		// (unparameterized) resources.
		var desc *workspace.PackageDescriptor
		if d, ok := descriptors[provider]; ok {
			desc = &d
		}

		tfType := res.Addr.Resource.Type
		info, err := providerInfoSource.GetProviderInfo(ctx, provider, desc)
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
		var (
			parameterization  *plugin.ResourceParameterization
			version           string
			pluginDownloadURL string
		)
		if desc != nil {
			parameterization, version = parameterizationFor(desc)
			pluginDownloadURL = desc.PluginDownloadURL
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
			resources = append(resources, plugin.ResourceImport{
				Type:              token,
				Name:              resourceName(res.Addr.Resource.Name, key),
				ID:                id,
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
// (pulumi-labs/pulumi-hcl#167).
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

// importID extracts the `id` attribute verbatim.
//
// TODO(pulumi-labs/pulumi-hcl#167): resources whose importer needs a
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

// parameterizationFor builds the replacement parameterization for a
// descriptor: the resource's Type/Version describe the parameterized package
// (e.g. "aws"), while the Parameterization block describes the base plugin it
// is produced from (e.g. "terraform-provider").
func parameterizationFor(desc *workspace.PackageDescriptor) (*plugin.ResourceParameterization, string) {
	if desc.Parameterization == nil {
		return nil, ""
	}
	var baseVersion string
	if desc.Version != nil {
		baseVersion = desc.Version.String()
	}
	return &plugin.ResourceParameterization{
		PluginName:    desc.Name,
		PluginVersion: baseVersion,
		Value:         desc.Parameterization.Value,
	}, desc.Parameterization.Version.String()
}
