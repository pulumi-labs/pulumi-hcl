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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	sdkschema "github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi/pulumi-hcl/pkg/util/encryption"
	"github.com/pulumi/pulumi-hcl/vendored/addrs"
	"github.com/pulumi/pulumi-hcl/vendored/statefile"
	"github.com/pulumi/pulumi-hcl/vendored/states"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	shimv2 "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim/sdk-v2"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeInfoSource is an in-memory bridge.ProviderInfoSource.
type fakeInfoSource struct {
	infos map[string]*tfbridge.ProviderInfo
}

func (f fakeInfoSource) GetProviderInfo(
	_ context.Context, tfProvider string, _ *workspace.PackageDescriptor,
) (*tfbridge.ProviderInfo, error) {
	return f.infos[tfProvider], nil
}

// roundtrip mirrors what the mapper delivers in production: the info is
// marshalled with its schema-only shim and unmarshalled back, so value
// translation runs against the same ProviderShim it would in the CLI.
func roundtrip(t *testing.T, info tfbridge.ProviderInfo) *tfbridge.ProviderInfo {
	t.Helper()
	b, err := json.Marshal(tfbridge.MarshalProviderInfo(&info))
	require.NoError(t, err)
	var m tfbridge.MarshallableProviderInfo
	require.NoError(t, json.Unmarshal(b, &m))
	return m.Unmarshal()
}

func awsInfoSource(t *testing.T) fakeInfoSource {
	t.Helper()
	p := &sdkschema.Provider{
		ResourcesMap: map[string]*sdkschema.Resource{
			"aws_s3_bucket": {
				Schema: map[string]*sdkschema.Schema{
					"acl":          {Type: sdkschema.TypeString, Optional: true},
					"secret_sauce": {Type: sdkschema.TypeString, Optional: true, Sensitive: true},
					"arn":          {Type: sdkschema.TypeString, Computed: true},
					"versioning": {
						Type: sdkschema.TypeList, MaxItems: 1, Optional: true,
						Elem: &sdkschema.Resource{Schema: map[string]*sdkschema.Schema{
							"enabled": {Type: sdkschema.TypeBool, Optional: true},
						}},
					},
				},
			},
		},
	}
	return fakeInfoSource{
		infos: map[string]*tfbridge.ProviderInfo{
			"aws": roundtrip(t, tfbridge.ProviderInfo{
				P:    shimv2.NewProvider(p),
				Name: "aws",
				Resources: map[string]*tfbridge.ResourceInfo{
					"aws_s3_bucket": {Tok: "aws:s3/bucket:Bucket"},
				},
			}),
		},
	}
}

// stateV4 wraps a `{"resources": [...]}` fragment in the v4 envelope the
// state-file parser requires.
func stateV4(t *testing.T, fragment string) []byte {
	t.Helper()
	var doc map[string]json.RawMessage
	require.NoError(t, json.Unmarshal([]byte(fragment), &doc))
	doc["version"] = json.RawMessage(`4`)
	doc["terraform_version"] = json.RawMessage(`"1.12.0"`)
	doc["lineage"] = json.RawMessage(`"00000000-0000-0000-0000-000000000000"`)
	doc["serial"] = json.RawMessage(`1`)
	full, err := json.Marshal(doc)
	require.NoError(t, err)
	return full
}

func parseState(t *testing.T, fragment string) *states.State {
	t.Helper()
	f, err := statefile.Read(bytes.NewReader(stateV4(t, fragment)), encryption.StateEncryptionDisabled())
	require.NoError(t, err)
	return f.State
}

func TestConvertTFState_EmitsImport(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "b",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "attributes": { "id": "my-bucket" } } ]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(t), state)

	require.Empty(t, resp.Diagnostics)
	require.Len(t, resp.Resources, 1)

	got := resp.Resources[0]
	assert.Equal(t, "aws:s3/bucket:Bucket", got.Type)
	assert.Equal(t, "b", got.Name)
	assert.Equal(t, "my-bucket", got.ID)
}

func TestConvertTFState_SkipsUnimportable(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "data", "type": "aws_ami", "name": "ubuntu",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "attributes": { "id": "ami-123" } } ]
			},
			{
				"module": "module.vpc", "mode": "managed", "type": "aws_s3_bucket", "name": "nested",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "attributes": { "id": "nested-bucket" } } ]
			},
			{
				"mode": "managed", "type": "gcp_storage_bucket", "name": "g",
				"provider": "provider[\"registry.terraform.io/hashicorp/gcp\"]",
				"instances": [ { "attributes": { "id": "gcp-bucket" } } ]
			},
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "no_id",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "attributes": { "acl": "private" } } ]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(t), state)

	// The data source skips silently; the other three each warn.
	assert.Empty(t, resp.Resources)
	assert.Len(t, resp.Diagnostics, 3)
	for _, d := range resp.Diagnostics {
		assert.Equal(t, hcl.DiagWarning, d.Severity)
	}
}

func TestConvertTFState_CountAndForEach(t *testing.T) {
	t.Parallel()

	// Emitted names must match the runtime's buildResourceName.
	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "counted",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [
					{ "index_key": 0, "attributes": { "id": "bucket-0" } },
					{ "index_key": 1, "attributes": { "id": "bucket-1" } }
				]
			},
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "byregion",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [
					{ "index_key": "east", "attributes": { "id": "bucket-east" } }
				]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(t), state)

	require.Empty(t, resp.Diagnostics)
	names := make(map[string]string, len(resp.Resources))
	for _, r := range resp.Resources {
		names[r.Name] = r.ID
	}
	assert.Equal(t, map[string]string{
		"counted[0]":       "bucket-0",
		"counted[1]":       "bucket-1",
		`byregion["east"]`: "bucket-east",
	}, names)
}

// A provider whose name does not prefix its resource types (google-beta
// owning google_* resources) must resolve via the state's provider address.
func TestConvertTFState_ProviderNamePrefixMismatch(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "confounding_provider_resource", "name": "sadness",
				"provider": "provider[\"registry.terraform.io/example/confounding-beta\"]",
				"instances": [ { "attributes": { "id": "c-1" } } ]
			}
		]
	}`)

	src := fakeInfoSource{infos: map[string]*tfbridge.ProviderInfo{
		"confounding-beta": roundtrip(t, tfbridge.ProviderInfo{
			P: shimv2.NewProvider(&sdkschema.Provider{
				ResourcesMap: map[string]*sdkschema.Resource{
					"confounding_provider_resource": {Schema: map[string]*sdkschema.Schema{
						"name": {Type: sdkschema.TypeString, Optional: true},
					}},
				},
			}),
			Name: "confounding-beta",
			Resources: map[string]*tfbridge.ResourceInfo{
				"confounding_provider_resource": {Tok: "confounding:index/resource:Resource"},
			},
		}),
	}}
	resp := convertTFState(t.Context(), src, state)

	require.Empty(t, resp.Diagnostics)
	require.Len(t, resp.Resources, 1)
	assert.Equal(t, "confounding:index/resource:Resource", resp.Resources[0].Type)
	assert.Equal(t, "c-1", resp.Resources[0].ID)
}

// TestConvertTFState_SuppliesValues pins the outputs-supplied import shape:
// bridge renames and MaxItemsOne flattening, schema-sensitive fields and
// dynamically-marked paths as secrets, and inputs reduced to the schema's
// input subset.
func TestConvertTFState_SuppliesValues(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "b",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ {
					"attributes": {
						"id": "my-bucket",
						"acl": "private",
						"secret_sauce": "hunter2",
						"arn": "arn:aws:s3:::my-bucket",
						"versioning": [ { "enabled": true } ]
					},
					"sensitive_attributes": [
						[ { "type": "get_attr", "value": "acl" } ],
						[
							{ "type": "get_attr", "value": "versioning" },
							{ "type": "index", "value": { "value": 0, "type": "number" } },
							{ "type": "get_attr", "value": "enabled" }
						]
					]
				} ]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(t), state)
	require.Empty(t, resp.Diagnostics)
	require.Len(t, resp.Resources, 1)
	got := resp.Resources[0]

	outs := got.Outputs
	assert.Equal(t, resource.NewStringProperty("my-bucket"), outs["id"])
	assert.Equal(t, resource.MakeSecret(resource.NewStringProperty("private")), outs["acl"],
		"dynamically-marked sensitive path")
	assert.Equal(t, resource.MakeSecret(resource.NewStringProperty("hunter2")), outs["secretSauce"],
		"schema-sensitive field")
	assert.Equal(t, resource.NewStringProperty("arn:aws:s3:::my-bucket"), outs["arn"])
	require.True(t, outs["versioning"].IsObject(), "MaxItemsOne block flattens to an object")
	assert.Equal(t, resource.MakeSecret(resource.NewBoolProperty(true)), outs["versioning"].ObjectValue()["enabled"],
		"the sensitive path's index step drops with the MaxItemsOne flattening")

	ins := got.Inputs
	assert.Contains(t, ins, resource.PropertyKey("acl"))
	assert.Contains(t, ins, resource.PropertyKey("secretSauce"))
	assert.Contains(t, ins, resource.PropertyKey("versioning"))
	assert.NotContains(t, ins, resource.PropertyKey("arn"), "computed-only fields are not inputs")
	assert.NotContains(t, ins, resource.PropertyKey("id"))
}

// TestConvertTFState_ValueFallbacks pins the degradations: instances whose
// values cannot be translated still import by id, with a warning.
func TestConvertTFState_ValueFallbacks(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "drifted",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "schema_version": 5, "attributes": { "id": "old-schema" } } ]
			},
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "legacy",
				"provider": "provider[\"registry.terraform.io/hashicorp/aws\"]",
				"instances": [ { "attributes_flat": { "id": "flatmap" } } ]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(t), state)
	require.Len(t, resp.Resources, 2)
	for _, r := range resp.Resources {
		assert.Nil(t, r.Outputs, "%s should import by id only", r.ID)
		assert.Nil(t, r.Inputs)
	}
	require.Len(t, resp.Diagnostics, 2)
	for _, d := range resp.Diagnostics {
		assert.Equal(t, "Importing without values", d.Summary)
	}
}

func TestResourceName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "b", resourceName("b", addrs.NoKey))
	assert.Equal(t, "b[0]", resourceName("b", addrs.IntKey(0)))
	assert.Equal(t, "b[2]", resourceName("b", addrs.IntKey(2)))
	assert.Equal(t, `b["east"]`, resourceName("b", addrs.StringKey("east")))
	assert.Equal(t, "my-bucket.v2", resourceName("my-bucket.v2", addrs.NoKey), "no sanitisation")
}

func TestImportID(t *testing.T) {
	t.Parallel()
	id, ok := importID(&states.ResourceInstanceObjectSrc{AttrsJSON: []byte(`{"id": "x"}`)})
	assert.True(t, ok)
	assert.Equal(t, "x", id)

	_, ok = importID(&states.ResourceInstanceObjectSrc{AttrsJSON: []byte(`{"acl": "private"}`)})
	assert.False(t, ok, "no id attribute")

	_, ok = importID(&states.ResourceInstanceObjectSrc{AttrsJSON: []byte(`{"id": ""}`)})
	assert.False(t, ok, "empty id")

	_, ok = importID(&states.ResourceInstanceObjectSrc{AttrsJSON: []byte(`{"id": 123}`)})
	assert.False(t, ok, "non-string id")

	id, ok = importID(&states.ResourceInstanceObjectSrc{AttrsFlat: map[string]string{"id": "flat"}})
	assert.True(t, ok, "pre-0.12 flatmap attributes")
	assert.Equal(t, "flat", id)
}

func TestConvertStateArgValidation(t *testing.T) {
	t.Parallel()

	_, err := New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		Args: []string{"state.json"},
	})
	assert.ErrorContains(t, err, "missing mapper target")

	_, err = New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		MapperTarget: "127.0.0.1:1",
	})
	assert.ErrorContains(t, err, "expected exactly one argument")

	_, err = New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		MapperTarget: "127.0.0.1:1",
		Args:         []string{"a", "b"},
	})
	assert.ErrorContains(t, err, "expected exactly one argument")
}

// staticMapper is a convert.Mapper with a fixed mapping for "random".
type staticMapper struct{}

func (staticMapper) GetMapping(
	_ context.Context, provider string, _ *convert.MapperPackageHint, _ string,
) ([]byte, error) {
	if provider != "random" {
		return nil, nil
	}
	return json.Marshal(tfbridge.MarshalProviderInfo(&tfbridge.ProviderInfo{
		P: shimv2.NewProvider(&sdkschema.Provider{
			ResourcesMap: map[string]*sdkschema.Resource{
				"random_uuid": {Schema: map[string]*sdkschema.Schema{
					"result": {Type: sdkschema.TypeString, Computed: true},
				}},
			},
		}),
		Name: "random",
		Resources: map[string]*tfbridge.ResourceInfo{
			"random_uuid": {Tok: "random:index/randomUuid:RandomUuid"},
		},
	}))
}

// TestConvertStateViaMapper drives the full converter entry point — gRPC mapper dialing
// and state-file parsing — against a real in-process mapper server.
func TestConvertStateViaMapper(t *testing.T) {
	t.Parallel()

	cancel := make(chan bool)
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancel,
		Init: func(srv *grpc.Server) error {
			convert.MapperRegistration(convert.NewMapperServer(staticMapper{}))(srv)
			return nil
		},
	})
	require.NoError(t, err)
	t.Cleanup(func() { close(cancel); <-handle.Done })
	target := fmt.Sprintf("127.0.0.1:%d", handle.Port)

	dir := t.TempDir()
	statePath := filepath.Join(dir, "terraform.tfstate")
	require.NoError(t, os.WriteFile(statePath, stateV4(t, `{
		"resources": [
			{
				"mode": "managed", "type": "random_uuid", "name": "example",
				"provider": "provider[\"registry.opentofu.org/hashicorp/random\"]",
				"instances": [ { "attributes": { "id": "aabbccdd-0011-2233-4455-66778899aabb" } } ]
			}
		]
	}`), 0o600))

	resp, err := New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		MapperTarget: target,
		Args:         []string{statePath},
	})
	require.NoError(t, err)
	require.Empty(t, resp.Diagnostics)
	require.Len(t, resp.Resources, 1)
	got := resp.Resources[0]
	assert.Equal(t, "random:index/randomUuid:RandomUuid", got.Type)
	assert.Equal(t, "example", got.Name)
	assert.Equal(t, "aabbccdd-0011-2233-4455-66778899aabb", got.ID)

	// State-file error paths share the same entry point.
	_, err = New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		MapperTarget: target,
		Args:         []string{filepath.Join(dir, "does-not-exist.tfstate")},
	})
	assert.ErrorContains(t, err, "reading state file")

	badPath := filepath.Join(dir, "bad.tfstate")
	require.NoError(t, os.WriteFile(badPath, []byte("not json"), 0o600))
	_, err = New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
		MapperTarget: target,
		Args:         []string{badPath},
	})
	assert.ErrorContains(t, err, "parsing state file")
}
