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
	"path/filepath"
	"testing"

	"github.com/google/go-cmp/cmp"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/testing/protocmp"
	"google.golang.org/protobuf/types/known/structpb"
)

func urnAlias(urn string) *pulumirpc.Alias {
	return &pulumirpc.Alias{Alias: &pulumirpc.Alias_Urn{Urn: urn}}
}

func specAlias(spec *pulumirpc.Alias_Spec) *pulumirpc.Alias {
	return &pulumirpc.Alias{Alias: &pulumirpc.Alias_Spec_{Spec: spec}}
}

func TestCollapseSpecAliases(t *testing.T) {
	t.Parallel()

	const (
		typ    = "conformance-component:index:Simple"
		name   = "res"
		parent = "urn:pulumi:stack::project::simple:index:Resource::parent"
	)

	cases := []struct {
		name  string
		alias *pulumirpc.Alias
		want  string
	}{
		{
			name:  "urn passthrough",
			alias: urnAlias("urn:pulumi:stack::project::a:b:C::old"),
			want:  "urn:pulumi:stack::project::a:b:C::old",
		},
		{
			name: "noParent",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Parent: &pulumirpc.Alias_Spec_NoParent{NoParent: true},
			}),
			want: "urn:pulumi:stack::project::conformance-component:index:Simple::res",
		},
		{
			name:  "empty spec inherits the current parent",
			alias: specAlias(&pulumirpc.Alias_Spec{}),
			want:  "urn:pulumi:stack::project::simple:index:Resource$conformance-component:index:Simple::res",
		},
		{
			name: "previous name",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Name:   "old",
				Parent: &pulumirpc.Alias_Spec_NoParent{NoParent: true},
			}),
			want: "urn:pulumi:stack::project::conformance-component:index:Simple::old",
		},
		{
			name: "previous type",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Type:   "pkg:index:Old",
				Parent: &pulumirpc.Alias_Spec_NoParent{NoParent: true},
			}),
			want: "urn:pulumi:stack::project::pkg:index:Old::res",
		},
		{
			name: "previous parent",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Parent: &pulumirpc.Alias_Spec_ParentUrn{
					ParentUrn: "urn:pulumi:stack::project::a:b:C::p",
				},
			}),
			want: "urn:pulumi:stack::project::a:b:C$conformance-component:index:Simple::res",
		},
		{
			name: "root stack parent is no parent",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Parent: &pulumirpc.Alias_Spec_ParentUrn{
					ParentUrn: "urn:pulumi:stack::project::pulumi:pulumi:Stack::project-stack",
				},
			}),
			want: "urn:pulumi:stack::project::conformance-component:index:Simple::res",
		},
		{
			name: "previous project and stack",
			alias: specAlias(&pulumirpc.Alias_Spec{
				Project: "proj2",
				Stack:   "stack2",
				Parent:  &pulumirpc.Alias_Spec_NoParent{NoParent: true},
			}),
			want: "urn:pulumi:stack2::proj2::conformance-component:index:Simple::res",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			got := collapseSpecAliases(
				[]*pulumirpc.Alias{tc.alias}, typ, name, parent, "project", "stack")
			require.Len(t, got, 1)
			assert.Equal(t, tc.want, got[0].GetUrn())
		})
	}
}

// TestConstructForwardsSpecAliases guards against dropping spec-form aliases
// on the component: the engine sends them on the ConstructRequest and expects
// the provider to attach the URNs they denote to the component's own
// RegisterResourceRequest, where the engine matches them against old state
// and inherits them onto the component's children.
// https://github.com/pulumi/pulumi-hcl/issues/543
func TestConstructForwardsSpecAliases(t *testing.T) {
	t.Parallel()

	dir, err := filepath.Abs(filepath.Join("testdata", "module-one-var"))
	require.NoError(t, err)

	prov, err := NewLocalProvider(t.Context(), dir, "127.0.0.1:1")
	require.NoError(t, err)
	_, err = prov.Handshake(t.Context(), &pulumirpc.ProviderHandshakeRequest{})
	require.NoError(t, err)

	mon := &captureMonitor{}
	endpoint := serveResourceMonitor(t, mon)
	inputs, err := structpb.NewStruct(map[string]any{"name": "world"})
	require.NoError(t, err)
	_, err = prov.Construct(t.Context(), &pulumirpc.ConstructRequest{
		Type:            "module-one-var:index:Module",
		Name:            "greet",
		Project:         "proj",
		Stack:           "test",
		MonitorEndpoint: endpoint,
		Inputs:          inputs,
		Aliases: []*pulumirpc.Alias{
			specAlias(&pulumirpc.Alias_Spec{
				Parent: &pulumirpc.Alias_Spec_NoParent{NoParent: true},
			}),
			urnAlias("urn:pulumi:test::proj::pkg:index:Old::previous"),
		},
		AcceptsOutputValues: true,
	})
	require.NoError(t, err)

	component := mon.registeredType("module-one-var:index:Module")
	require.NotNil(t, component, "the component itself must be registered")
	assert.Empty(t, cmp.Diff([]*pulumirpc.Alias{
		urnAlias("urn:pulumi:test::proj::module-one-var:index:Module::greet"),
		urnAlias("urn:pulumi:test::proj::pkg:index:Old::previous"),
	}, component.Aliases, protocmp.Transform()))
}
