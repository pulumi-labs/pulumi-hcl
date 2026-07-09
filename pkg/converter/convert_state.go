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
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
)

// tfState is a minimal view over a Terraform/OpenTofu v4 JSON state file;
// only the fields ConvertState needs are decoded.
type tfState struct {
	Resources []tfStateResource `json:"resources"`
}

type tfStateResource struct {
	Module    string            `json:"module"`
	Mode      string            `json:"mode"`
	Type      string            `json:"type"`
	Name      string            `json:"name"`
	Instances []tfStateInstance `json:"instances"`

	// The state's `provider` field (the canonical provider address) is
	// deliberately not decoded: providers resolve from the type's prefix
	// (see providerLocalName) because that is how the runtime's resolver
	// keys mapper lookups, and an import must resolve types the same way
	// the program will.
}

type tfStateInstance struct {
	// IndexKey is null for a singleton, a number for `count`, or a string
	// for `for_each`.
	IndexKey   any                        `json:"index_key"`
	Attributes map[string]json.RawMessage `json:"attributes"`
}

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

	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, fmt.Errorf("reading state file %q: %w", statePath, err)
	}
	var state tfState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, fmt.Errorf("parsing state file %q: %w", statePath, err)
	}

	return convertTFState(ctx, providerInfoSource, state), nil
}

// convertTFState is the pure core of ConvertState: it turns a parsed state file
// into ResourceImports given a resolved provider-info source. Anything that
// can't be imported is reported as a warning diagnostic rather than failing the
// whole conversion.
func convertTFState(
	ctx context.Context,
	providerInfoSource bridge.ProviderInfoSource,
	state tfState,
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

	for _, res := range state.Resources {
		if res.Mode != "managed" {
			continue
		}
		// Module-nested resources are skipped: core can import components
		// (pulumi/pulumi#15199), but mapping module instances to the component
		// names the HCL runtime registers is not built yet.
		if res.Module != "" {
			warn("Skipped module resource", fmt.Sprintf(
				"resource %q is nested in %q; module import is not yet supported", res.Name, res.Module))
			continue
		}

		provider := providerLocalName(res.Type)
		info, err := providerInfoSource.GetProviderInfo(ctx, provider, nil)
		if err != nil || info == nil {
			warn("Failed to resolve provider", fmt.Sprintf(
				"could not resolve bridge mapping for provider %q: %v", provider, err))
			continue
		}
		resInfo, ok := info.Resources[res.Type]
		if !ok || resInfo == nil || resInfo.Tok == "" {
			warn("Failed to resolve resource type", fmt.Sprintf(
				"provider %q has no mapping for TF type %q", provider, res.Type))
			continue
		}
		token := resInfo.Tok.String()

		for _, inst := range res.Instances {
			id, ok := importID(inst)
			if !ok {
				warn("Skipped resource without id", fmt.Sprintf(
					"an instance of %s.%s has no string `id` attribute to import by", res.Type, res.Name))
				continue
			}
			resources = append(resources, plugin.ResourceImport{
				Type: token,
				Name: resourceName(res.Name, inst.IndexKey),
				ID:   id,
			})
		}
	}

	return &plugin.ConvertStateResponse{Resources: resources, Diagnostics: diagnostics}
}

// providerLocalName derives the TF provider local name from a resource type
// the same way the runtime resolver's providerInfoForType does; single-segment
// types (e.g. "external") are their own provider name.
func providerLocalName(tfType string) string {
	if i := strings.IndexByte(tfType, '_'); i > 0 {
		return tfType[:i]
	}
	return tfType
}

// resourceName reproduces the HCL runtime's buildResourceName so imported
// resources land on the URNs the program will register: the modulepath step
// naming for expanded instances, and deliberately no sanitisation, unlike
// pulumi-converter-terraform — the runtime registers raw labels, so
// sanitising here would diff.
func resourceName(name string, indexKey any) string {
	switch k := indexKey.(type) {
	case float64:
		return modulepath.NewIndexedStep(name, int(k)).LogicalName()
	case string:
		return modulepath.NewKeyedStep(name, k).LogicalName()
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
func importID(inst tfStateInstance) (string, bool) {
	raw, ok := inst.Attributes["id"]
	if !ok {
		return "", false
	}
	var id string
	if err := json.Unmarshal(raw, &id); err != nil || id == "" {
		return "", false
	}
	return id, true
}
