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
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	shimv2 "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim/sdk-v2"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// synInfos builds a live bridged provider — synthesis needs a working shim,
// unlike ConvertState which only consults marshaled mappings.
func synInfos() map[string]tfbridge.ProviderInfo {
	p := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"syn_thing": {
				Schema: map[string]*schema.Schema{
					"name":         {Type: schema.TypeString, Optional: true},
					"secret_val":   {Type: schema.TypeString, Optional: true, Sensitive: true},
					"computed_val": {Type: schema.TypeString, Computed: true},
				},
			},
		},
	}
	return map[string]tfbridge.ProviderInfo{
		"syn": {
			P:    shimv2.NewProvider(p),
			Name: "syn",
			Resources: map[string]*tfbridge.ResourceInfo{
				"syn_thing": {Tok: "syn:index/thing:Thing"},
			},
		},
	}
}

func writeStateFile(t *testing.T, stateJSON string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "terraform.tfstate")
	require.NoError(t, os.WriteFile(path, []byte(stateJSON), 0o600))
	return path
}

func synthesize(t *testing.T, projectDir, stateJSON string) apitype.DeploymentV3 {
	t.Helper()
	dep, diags, err := SynthesizeStateDeployment(
		t.Context(), synInfos(), writeStateFile(t, stateJSON), projectDir, "proj", "stack")
	require.NoError(t, err)
	require.Empty(t, diags)
	require.EqualValues(t, apitype.DeploymentSchemaVersionCurrent, dep.Version)
	var deployment apitype.DeploymentV3
	require.NoError(t, json.Unmarshal(dep.Deployment, &deployment))
	return deployment
}

func TestSynthesizeState_TranslatesAttributes(t *testing.T) {
	t.Parallel()

	deployment := synthesize(t, t.TempDir(), `{
		"resources": [
			{
				"mode": "managed", "type": "syn_thing", "name": "a",
				"instances": [ { "schema_version": 2, "attributes": {
					"id": "thing-1", "name": "x", "secret_val": "hunter2", "computed_val": "out"
				} } ]
			}
		]
	}`)

	require.Len(t, deployment.Resources, 1)
	got := deployment.Resources[0]
	assert.Equal(t, "urn:pulumi:stack::proj::syn:index/thing:Thing::a", string(got.URN))
	assert.Equal(t, "syn:index/thing:Thing", string(got.Type))
	assert.True(t, got.Custom)
	assert.Equal(t, "thing-1", string(got.ID))
	// No parameterization descriptor, so no provider entry: the engine
	// injects the default provider on the next load.
	assert.Empty(t, got.Provider)

	assert.Equal(t, "x", got.Outputs["name"])
	assert.Equal(t, "out", got.Outputs["computedVal"])
	assert.JSONEq(t, `{"schema_version":"2"}`, got.Outputs["__meta"].(string))
	// Sensitive attributes come across as plaintext secrets; `pulumi stack
	// import` re-encrypts them.
	secret, ok := got.Outputs["secretVal"].(map[string]any)
	require.True(t, ok, "secretVal should serialize as a secret, got %#v", got.Outputs["secretVal"])
	assert.Equal(t, `"hunter2"`, secret["plaintext"])

	assert.Equal(t, "x", got.Inputs["name"])
	assert.NotContains(t, got.Inputs, "computedVal", "pure outputs are not inputs")
}

func TestSynthesizeState_ParameterizedProvider(t *testing.T) {
	t.Parallel()

	projectDir := t.TempDir()
	desc, err := json.Marshal(awsDescriptor())
	require.NoError(t, err)
	sdkDir := filepath.Join(projectDir, "sdks", "syn")
	require.NoError(t, os.MkdirAll(sdkDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(sdkDir, "hcl.sdk.json"), desc, 0o600))

	deployment := synthesize(t, projectDir, `{
		"resources": [
			{
				"mode": "managed", "type": "syn_thing", "name": "a",
				"instances": [ { "attributes": { "id": "thing-1", "name": "x" } } ]
			}
		]
	}`)

	// The parameterized default provider precedes its resources, as checkpoint
	// integrity checking requires.
	require.Len(t, deployment.Resources, 2)
	prov, res := deployment.Resources[0], deployment.Resources[1]

	assert.Equal(t, "urn:pulumi:stack::proj::pulumi:providers:aws::default_6_0_0", string(prov.URN))
	assert.True(t, prov.Custom)
	assert.NotEmpty(t, prov.ID)
	assert.Equal(t, "6.0.0", prov.Inputs["version"])
	internal, ok := prov.Inputs["__internal"].(map[string]any)
	require.True(t, ok)
	assert.Equal(t, "terraform-provider", internal["name"])
	assert.Equal(t, "0.0.1", internal["version"])
	assert.Equal(t, "github://api.github.com/pulumi/pulumi-terraform-provider", internal["pluginDownloadURL"])
	assert.NotEmpty(t, internal["parameterization"])

	assert.Equal(t, string(prov.URN)+"::"+string(prov.ID), res.Provider)
}

func TestSynthesizeState_SkipsAndWarns(t *testing.T) {
	t.Parallel()

	dep, diags, err := SynthesizeStateDeployment(t.Context(), synInfos(), writeStateFile(t, `{
		"resources": [
			{
				"mode": "managed", "type": "syn_thing", "name": "nested", "module": "module.m",
				"instances": [ { "attributes": { "id": "thing-1" } } ]
			},
			{
				"mode": "managed", "type": "syn_thing", "name": "no_id",
				"instances": [ { "attributes": { "name": "x" } } ]
			},
			{
				"mode": "managed", "type": "syn_unmapped", "name": "unmapped",
				"instances": [ { "attributes": { "id": "thing-2" } } ]
			},
			{
				"mode": "managed", "type": "other_thing", "name": "unknown_provider",
				"instances": [ { "attributes": { "id": "thing-3" } } ]
			},
			{
				"mode": "data", "type": "syn_thing", "name": "datasource",
				"instances": [ { "attributes": { "id": "thing-4" } } ]
			}
		]
	}`), t.TempDir(), "proj", "stack")
	require.NoError(t, err)

	var deployment apitype.DeploymentV3
	require.NoError(t, json.Unmarshal(dep.Deployment, &deployment))
	assert.Empty(t, deployment.Resources)

	summaries := make([]string, len(diags))
	for i, d := range diags {
		require.Equal(t, hcl.DiagWarning, d.Severity)
		summaries[i] = d.Summary
	}
	assert.Equal(t, []string{
		"Skipped module resource",
		"Skipped resource without id",
		"Failed to resolve resource type",
		"Failed to resolve provider",
	}, summaries)
}

func TestSynthesizeState_FileErrors(t *testing.T) {
	t.Parallel()

	_, _, err := SynthesizeStateDeployment(
		t.Context(), synInfos(), filepath.Join(t.TempDir(), "missing.tfstate"), t.TempDir(), "proj", "stack")
	assert.ErrorContains(t, err, "reading state file")

	_, _, err = SynthesizeStateDeployment(
		t.Context(), synInfos(), writeStateFile(t, "not json"), t.TempDir(), "proj", "stack")
	assert.ErrorContains(t, err, "parsing state file")
}
