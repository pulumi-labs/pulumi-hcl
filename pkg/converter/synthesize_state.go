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
	"encoding/base64"
	"encoding/json"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"

	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	shim "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim"
	pkgresource "github.com/pulumi/pulumi/pkg/v3/resource"
	"github.com/pulumi/pulumi/pkg/v3/resource/stack"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/config"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
)

// SynthesizeStateDeployment converts a TF state file into a Pulumi deployment
// for `pulumi stack import --file`, copying resource attributes into Pulumi
// state directly instead of importing by ID. Where ConvertState is bounded by
// each resource's importer (composite IDs, resources without importers,
// attributes the backend cannot return), synthesis carries the full attribute
// set across, offline: no provider Read runs.
//
// Unlike ConvertState this cannot run behind the converter plugin interface —
// plugin.ConvertStateResponse has no channel for property values — and it
// needs live provider schemas (tfbridge.ProviderInfo with a working shim), so
// it takes those directly instead of a mapper target.
//
// Outputs are the fidelity-critical half: on the next preview the bridge
// rebuilds TF state from them, so they must be the exact bridge projection of
// the TF attributes; MakeTerraformResult guarantees that. Inputs only need to
// be Check-stable, and are approximated from the outputs the same way the
// bridge's own import path does.
func SynthesizeStateDeployment(
	ctx context.Context,
	infos map[string]tfbridge.ProviderInfo,
	statePath, projectDir, project, stackName string,
) (*apitype.UntypedDeployment, hcl.Diagnostics, error) {
	descriptors, err := readParameterizationInfos(projectDir)
	if err != nil {
		return nil, nil, fmt.Errorf("reading parameterization infos: %w", err)
	}
	data, err := os.ReadFile(statePath)
	if err != nil {
		return nil, nil, fmt.Errorf("reading state file %q: %w", statePath, err)
	}
	var state tfState
	if err := json.Unmarshal(data, &state); err != nil {
		return nil, nil, fmt.Errorf("parsing state file %q: %w", statePath, err)
	}

	var diagnostics hcl.Diagnostics
	warn := func(summary, detail string) {
		diagnostics = append(diagnostics, &hcl.Diagnostic{
			Severity: hcl.DiagWarning,
			Summary:  summary,
			Detail:   detail,
		})
	}

	// Parameterized providers must be written into the deployment explicitly:
	// the engine injects default providers for resources without a provider
	// reference on the next load, but without parameterization, so it would
	// try to boot the parameterized package name as a plugin. Plain providers
	// are left to that injection. VerifyIntegrity requires providers to
	// precede the resources referencing them.
	var states []*pkgresource.State
	providerRefs := map[string]string{}
	for _, provider := range usedProviders(state) {
		desc, ok := descriptors[provider]
		if !ok || desc.Parameterization == nil {
			continue
		}
		st, ref := synthesizeProviderState(desc, project, stackName)
		states = append(states, st)
		providerRefs[provider] = ref
	}

	for _, res := range state.Resources {
		if res.Mode != "managed" {
			continue
		}
		if res.Module != "" {
			warn("Skipped module resource", fmt.Sprintf(
				"resource %q is nested in %q; module import is not yet supported", res.Name, res.Module))
			continue
		}

		provider := providerLocalName(res.Type)
		info, ok := infos[provider]
		if !ok {
			warn("Failed to resolve provider", fmt.Sprintf(
				"no bridged provider info supplied for provider %q", provider))
			continue
		}
		resInfo, ok := info.Resources[res.Type]
		if !ok || resInfo == nil || resInfo.Tok == "" {
			warn("Failed to resolve resource type", fmt.Sprintf(
				"provider %q has no mapping for TF type %q", provider, res.Type))
			continue
		}
		shimRes, ok := info.P.ResourcesMap().GetOk(res.Type)
		if !ok {
			warn("Failed to resolve resource schema", fmt.Sprintf(
				"provider %q has no schema for TF type %q", provider, res.Type))
			continue
		}

		for _, inst := range res.Instances {
			id, ok := importID(inst)
			if !ok {
				warn("Skipped resource without id", fmt.Sprintf(
					"an instance of %s.%s has no string `id` attribute", res.Type, res.Name))
				continue
			}
			st, err := synthesizeResourceState(
				ctx, info, resInfo, shimRes, res, inst, id, project, stackName, providerRefs[provider])
			if err != nil {
				warn("Failed to synthesize resource state", fmt.Sprintf(
					"instance %q of %s.%s: %v", id, res.Type, res.Name, err))
				continue
			}
			states = append(states, st)
		}
	}

	resources := make([]apitype.ResourceV3, 0, len(states))
	for _, st := range states {
		// showSecrets serializes secrets as plaintext; `pulumi stack import`
		// re-encrypts them under the target stack's secrets manager.
		v3, _, err := stack.SerializeResource(ctx, st, config.NopEncrypter, true)
		if err != nil {
			return nil, diagnostics, fmt.Errorf("serializing %s: %w", st.URN, err)
		}
		resources = append(resources, v3)
	}
	deployment, err := json.Marshal(apitype.DeploymentV3{Resources: resources})
	if err != nil {
		return nil, diagnostics, fmt.Errorf("marshaling deployment: %w", err)
	}
	return &apitype.UntypedDeployment{
		Version:    apitype.DeploymentSchemaVersionCurrent,
		Deployment: deployment,
	}, diagnostics, nil
}

// synthesizeResourceState builds the Pulumi state entry for one TF resource
// instance by round-tripping its attributes through the bridge's own
// TF-state → Pulumi translation.
func synthesizeResourceState(
	ctx context.Context,
	info tfbridge.ProviderInfo,
	resInfo *tfbridge.ResourceInfo,
	shimRes shim.Resource,
	res tfStateResource,
	inst tfStateInstance,
	id, project, stackName, providerRef string,
) (*pkgresource.State, error) {
	attrs, err := decodeAttributes(inst.Attributes)
	if err != nil {
		return nil, err
	}
	// The bridge reads schema_version back from __meta as a decimal string;
	// without it, synthesized state would be upgraded from version 0.
	meta := map[string]any{}
	if inst.SchemaVersion > 0 {
		meta["schema_version"] = strconv.FormatUint(inst.SchemaVersion, 10)
	}
	tfState, err := shimRes.InstanceState(id, attrs, meta)
	if err != nil {
		return nil, fmt.Errorf("building instance state: %w", err)
	}

	outs, err := tfbridge.MakeTerraformResult(
		ctx, info.P, tfState, shimRes.Schema(), resInfo.Fields, nil, true)
	if err != nil {
		return nil, fmt.Errorf("translating outputs: %w", err)
	}
	ins, err := tfbridge.ExtractInputsFromOutputs(nil, outs, shimRes.Schema(), resInfo.Fields, false)
	if err != nil {
		return nil, fmt.Errorf("deriving inputs: %w", err)
	}

	name := resourceName(res.Name, inst.IndexKey)
	urn := resource.NewURN(tokens.QName(stackName), tokens.PackageName(project), "", resInfo.Tok, name)
	return &pkgresource.State{
		Type:     resInfo.Tok,
		URN:      urn,
		Custom:   true,
		ID:       resource.ID(id),
		Inputs:   ins,
		Outputs:  outs,
		Provider: providerRef,
	}, nil
}

// usedProviders returns the sorted provider local names of the state's
// importable resources.
func usedProviders(state tfState) []string {
	set := map[string]bool{}
	for _, res := range state.Resources {
		if res.Mode == "managed" && res.Module == "" {
			set[providerLocalName(res.Type)] = true
		}
	}
	providers := make([]string, 0, len(set))
	for p := range set {
		providers = append(providers, p)
	}
	sort.Strings(providers)
	return providers
}

// synthesizeProviderState builds the state entry for a parameterized default
// provider, mirroring what the engine writes when the runtime registers the
// package: the resource type and root version name the parameterized package,
// while __internal carries the base plugin identity, exactly as the engine's
// provider registry reads it back.
func synthesizeProviderState(
	desc workspace.PackageDescriptor, project, stackName string,
) (*pkgresource.State, string) {
	pkg := desc.Parameterization.Name
	pkgVersion := desc.Parameterization.Version.String()
	// The engine names default providers after the requested version; using
	// the same name lets the next operation match this entry instead of
	// creating a second default provider.
	name := "default_" + strings.ReplaceAll(pkgVersion, ".", "_")
	typ := tokens.Type("pulumi:providers:" + pkg)
	urn := resource.NewURN(tokens.QName(stackName), tokens.PackageName(project), "", typ, name)
	id := uuid.NewSHA1(uuid.NameSpaceURL, []byte("pulumi-hcl-state-synthesis:"+pkg)).String()

	internal := resource.PropertyMap{
		"name":             resource.NewStringProperty(desc.Name),
		"parameterization": resource.NewStringProperty(base64.StdEncoding.EncodeToString(desc.Parameterization.Value)),
	}
	if desc.Version != nil {
		internal["version"] = resource.NewStringProperty(desc.Version.String())
	}
	if desc.PluginDownloadURL != "" {
		internal["pluginDownloadURL"] = resource.NewStringProperty(desc.PluginDownloadURL)
	}
	st := &pkgresource.State{
		Type:   typ,
		URN:    urn,
		Custom: true,
		ID:     resource.ID(id),
		Inputs: resource.PropertyMap{
			"version":    resource.NewStringProperty(pkgVersion),
			"__internal": resource.NewObjectProperty(internal),
		},
	}
	return st, string(urn) + "::" + id
}

// decodeAttributes decodes the instance's raw attribute values. Numbers
// decode as float64: the shim's InstanceState coercion accepts those but
// panics on json.Number.
func decodeAttributes(raw map[string]json.RawMessage) (map[string]any, error) {
	attrs := make(map[string]any, len(raw))
	for k, v := range raw {
		var out any
		if err := json.Unmarshal(v, &out); err != nil {
			return nil, fmt.Errorf("decoding attribute %q: %w", k, err)
		}
		attrs[k] = out
	}
	return attrs, nil
}
