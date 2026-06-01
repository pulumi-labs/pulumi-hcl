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

package bridge_test

import (
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	shimv2 "github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfshim/sdk-v2"
	"github.com/stretchr/testify/require"
)

func newTestProvider(t *testing.T, sdk *schema.Provider, overrides map[string]*tfbridge.ResourceInfo) *tfbridge.ProviderInfo {
	t.Helper()
	require.NoError(t, sdk.InternalValidate())
	return &tfbridge.ProviderInfo{
		P:         shimv2.NewProvider(sdk),
		Name:      "test",
		Version:   "0.0.1",
		Resources: overrides,
	}
}

func TestResourceBodyMapping_SingularBlockFlattenedToObject(t *testing.T) {
	t.Parallel()
	sdk := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"test_thing": {
				Schema: map[string]*schema.Schema{
					"settings": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: 1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"mode": {Type: schema.TypeString, Required: true},
							},
						},
					},
				},
			},
		},
	}
	info := newTestProvider(t, sdk, nil)
	m := bridge.ResourceBodyMapping(info, "test_thing")
	require.NotNil(t, m)

	settings := m.Lookup("settings")
	require.NotNil(t, settings)
	require.Equal(t, &bridge.FieldMapping{
		TFName:      "settings",
		PulumiName:  "settings",
		TFBlock:     true,
		MaxItemsOne: true,
		Nested:      settings.Nested, // verified next
	}, settings)

	require.NotNil(t, settings.Nested)
	mode := settings.Nested.Lookup("mode")
	require.NotNil(t, mode)
	require.False(t, mode.TFBlock, "mode is a scalar attribute, not a block")
	require.Equal(t, "mode", mode.PulumiName)
}

func TestResourceBodyMapping_RepeatedBlockProjectedAsList(t *testing.T) {
	t.Parallel()
	sdk := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"test_thing": {
				Schema: map[string]*schema.Schema{
					"tag": {
						Type:     schema.TypeList,
						Optional: true,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"key":   {Type: schema.TypeString, Required: true},
								"value": {Type: schema.TypeString, Required: true},
							},
						},
					},
				},
			},
		},
	}
	info := newTestProvider(t, sdk, nil)
	m := bridge.ResourceBodyMapping(info, "test_thing")
	require.NotNil(t, m)

	tag := m.Lookup("tag")
	require.NotNil(t, tag)
	require.True(t, tag.TFBlock)
	require.False(t, tag.MaxItemsOne, "no MaxItems set, so the bridge keeps it as a list")
}

func TestResourceBodyMapping_ExplicitMaxItemsOneOverride(t *testing.T) {
	t.Parallel()
	sdk := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"test_thing": {
				Schema: map[string]*schema.Schema{
					// Schema would default to a list (MaxItems=0).
					"settings": {
						Type:     schema.TypeList,
						Optional: true,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"mode": {Type: schema.TypeString, Required: true},
							},
						},
					},
				},
			},
		},
	}
	one := true
	overrides := map[string]*tfbridge.ResourceInfo{
		"test_thing": {Fields: map[string]*tfbridge.SchemaInfo{
			"settings": {MaxItemsOne: &one},
		}},
	}
	info := newTestProvider(t, sdk, overrides)
	m := bridge.ResourceBodyMapping(info, "test_thing")
	require.NotNil(t, m)

	settings := m.Lookup("settings")
	require.NotNil(t, settings)
	require.True(t, settings.MaxItemsOne, "explicit override beats schema MaxItems")
}

func TestResourceBodyMapping_NestedBlocks(t *testing.T) {
	t.Parallel()
	sdk := &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"test_thing": {
				Schema: map[string]*schema.Schema{
					"policy": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: 1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"effect": {Type: schema.TypeString, Required: true},
								"rule": {
									Type:     schema.TypeList,
									Optional: true,
									MaxItems: 1,
									Elem: &schema.Resource{
										Schema: map[string]*schema.Schema{
											"action": {Type: schema.TypeString, Required: true},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	info := newTestProvider(t, sdk, nil)
	m := bridge.ResourceBodyMapping(info, "test_thing")
	require.NotNil(t, m)

	policy := m.Lookup("policy")
	require.NotNil(t, policy)
	require.True(t, policy.TFBlock)
	require.True(t, policy.MaxItemsOne)

	require.NotNil(t, policy.Nested)
	rule := policy.Nested.Lookup("rule")
	require.NotNil(t, rule)
	require.True(t, rule.TFBlock)
	require.True(t, rule.MaxItemsOne)

	require.NotNil(t, rule.Nested)
	require.NotNil(t, rule.Nested.Lookup("action"))
	require.False(t, rule.Nested.Lookup("action").TFBlock)
}

func TestResourceBodyMapping_MissingResourceReturnsNil(t *testing.T) {
	t.Parallel()
	sdk := &schema.Provider{ResourcesMap: map[string]*schema.Resource{}}
	info := newTestProvider(t, sdk, nil)
	require.Nil(t, bridge.ResourceBodyMapping(info, "test_thing"))
}

func TestResourceBodyMapping_NilProviderInfoReturnsNil(t *testing.T) {
	t.Parallel()
	require.Nil(t, bridge.ResourceBodyMapping(nil, "any"))
}
