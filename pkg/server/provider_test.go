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

package server

import (
	"encoding/json"
	"path/filepath"
	"testing"

	pulumiSchema "github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/structpb"
)

// TestNewLocalProvider drives the locally-born provider — the RunPlugin path —
// end to end over the raw gRPC surface. The loader/mapper address points at an
// unused port: both clients dial lazily, and a provider-free module never uses
// them.
func TestNewLocalProvider(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(filepath.Join("testdata", "module-one-var"))
	require.NoError(t, err)

	prov, err := NewLocalProvider(t.Context(), dir, "127.0.0.1:1")
	require.NoError(t, err)

	// An address-free handshake, as the RunPlugin flow sends, must succeed.
	_, err = prov.Handshake(t.Context(), &pulumirpc.ProviderHandshakeRequest{})
	require.NoError(t, err)

	info, err := prov.GetPluginInfo(t.Context(), &emptypb.Empty{})
	require.NoError(t, err)
	require.Equal(t, "0.0.0-dev", info.Version)

	out, err := prov.GetSchema(t.Context(), &pulumirpc.GetSchemaRequest{})
	require.NoError(t, err)
	var spec pulumiSchema.PackageSpec
	require.NoError(t, json.Unmarshal([]byte(out.Schema), &spec))
	require.Equal(t, "module-one-var", spec.Name)
	require.Nil(t, spec.Parameterization)
	res, ok := spec.Resources["module-one-var:index:Module"]
	require.True(t, ok, "schema should declare the typed component")
	require.True(t, res.IsComponent)
	require.Equal(t, "string", res.InputProperties["name"].Type)
	require.Equal(t, "string", res.Properties["greeting"].Type)

	mon, _, endpoint := serveMonitor(t)
	inputs, err := structpb.NewStruct(map[string]any{"name": "world"})
	require.NoError(t, err)
	resp, err := prov.Construct(t.Context(), &pulumirpc.ConstructRequest{
		Type:                "module-one-var:index:Module",
		Name:                "greet",
		Project:             "proj",
		Stack:               "test",
		Organization:        "acme",
		MonitorEndpoint:     endpoint,
		Inputs:              inputs,
		ReplaceWith:         []string{"urn:pulumi:test::proj::pkg:index:Other::sibling"},
		ReplacementTrigger:  structpb.NewStringValue("trigger"),
		AcceptsOutputValues: true,
	})
	require.NoError(t, err)
	require.Equal(t, "hello world", resp.State.Fields["greeting"].GetStringValue())

	// The component registration carries the forwarded resource options.
	component := mon.registeredType("module-one-var:index:Module")
	require.NotNil(t, component, "the component itself must be registered")
	require.Equal(t, []string{"urn:pulumi:test::proj::pkg:index:Other::sibling"}, component.ReplaceWith)
	require.Equal(t, structpb.NewStringValue("trigger"), component.ReplacementTrigger)
}
