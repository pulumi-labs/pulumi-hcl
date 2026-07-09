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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// fakeInfoSource is an in-memory bridge.ProviderInfoSource.
type fakeInfoSource struct {
	infos map[string]*tfbridge.ProviderInfo
	errs  map[string]error
}

func (f fakeInfoSource) GetProviderInfo(
	_ context.Context, tfProvider string, _ *workspace.PackageDescriptor,
) (*tfbridge.ProviderInfo, error) {
	if err, ok := f.errs[tfProvider]; ok {
		return nil, err
	}
	return f.infos[tfProvider], nil
}

func awsInfoSource() fakeInfoSource {
	return fakeInfoSource{
		infos: map[string]*tfbridge.ProviderInfo{
			"aws": {
				Name: "aws",
				Resources: map[string]*tfbridge.ResourceInfo{
					"aws_s3_bucket": {Tok: "aws:s3/bucket:Bucket"},
				},
			},
		},
	}
}

func parseState(t *testing.T, stateJSON string) tfState {
	t.Helper()
	var state tfState
	require.NoError(t, json.Unmarshal([]byte(stateJSON), &state))
	return state
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

	resp := convertTFState(t.Context(), awsInfoSource(), state)

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
				"instances": [ { "attributes": { "id": "ami-123" } } ]
			},
			{
				"module": "module.vpc", "mode": "managed", "type": "aws_s3_bucket", "name": "nested",
				"instances": [ { "attributes": { "id": "nested-bucket" } } ]
			},
			{
				"mode": "managed", "type": "gcp_storage_bucket", "name": "g",
				"instances": [ { "attributes": { "id": "gcp-bucket" } } ]
			},
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "no_id",
				"instances": [ { "attributes": { "acl": "private" } } ]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(), state)

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
				"instances": [
					{ "index_key": 0, "attributes": { "id": "bucket-0" } },
					{ "index_key": 1, "attributes": { "id": "bucket-1" } }
				]
			},
			{
				"mode": "managed", "type": "aws_s3_bucket", "name": "byregion",
				"instances": [
					{ "index_key": "east", "attributes": { "id": "bucket-east" } }
				]
			}
		]
	}`)

	resp := convertTFState(t.Context(), awsInfoSource(), state)

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

// TestConvertTFState_UnderscoreProviderName documents the resolution limit
// for providers whose local name itself contains an underscore: the resource
// type "confounding_provider_resource" resolves to provider "confounding",
// not "confounding_provider", so its resources warn and skip. This mirrors
// the runtime resolver (packages.Resolver.providerInfoForType splits on the
// first underscore), which cannot run such programs either — import and
// runtime fail together rather than diverge.
func TestConvertTFState_UnderscoreProviderName(t *testing.T) {
	t.Parallel()

	state := parseState(t, `{
		"resources": [
			{
				"mode": "managed", "type": "confounding_provider_resource", "name": "sadness",
				"instances": [ { "attributes": { "id": "c-1" } } ]
			}
		]
	}`)

	src := fakeInfoSource{infos: map[string]*tfbridge.ProviderInfo{
		"confounding_provider": {
			Name: "confounding_provider",
			Resources: map[string]*tfbridge.ResourceInfo{
				"confounding_provider_resource": {Tok: "confounding:index/resource:Resource"},
			},
		},
	}}
	resp := convertTFState(t.Context(), src, state)

	assert.Empty(t, resp.Resources)
	require.Len(t, resp.Diagnostics, 1)
	assert.Equal(t, "Failed to resolve provider", resp.Diagnostics[0].Summary)
	assert.Contains(t, resp.Diagnostics[0].Detail, `"confounding"`)
}

func TestProviderLocalName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "aws", providerLocalName("aws_s3_bucket"))
	assert.Equal(t, "aws", providerLocalName("aws_s3_bucket_object"))
	assert.Equal(t, "external", providerLocalName("external"))
}

func TestResourceName(t *testing.T) {
	t.Parallel()
	assert.Equal(t, "b", resourceName("b", nil))
	assert.Equal(t, "b[0]", resourceName("b", float64(0)))
	assert.Equal(t, "b[2]", resourceName("b", float64(2)))
	assert.Equal(t, `b["east"]`, resourceName("b", "east"))
	assert.Equal(t, "my-bucket.v2", resourceName("my-bucket.v2", nil), "no sanitisation")
}

func TestImportID(t *testing.T) {
	t.Parallel()
	id, ok := importID(tfStateInstance{Attributes: map[string]json.RawMessage{"id": json.RawMessage(`"x"`)}})
	assert.True(t, ok)
	assert.Equal(t, "x", id)

	_, ok = importID(tfStateInstance{Attributes: map[string]json.RawMessage{"acl": json.RawMessage(`"private"`)}})
	assert.False(t, ok, "no id attribute")

	_, ok = importID(tfStateInstance{Attributes: map[string]json.RawMessage{"id": json.RawMessage(`""`)}})
	assert.False(t, ok, "empty id")

	_, ok = importID(tfStateInstance{Attributes: map[string]json.RawMessage{"id": json.RawMessage(`123`)}})
	assert.False(t, ok, "non-string id")
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
	require.NoError(t, os.WriteFile(statePath, []byte(`{
		"resources": [
			{
				"mode": "managed", "type": "random_uuid", "name": "example",
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
