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

package providers

import (
	"context"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type renamedProviderMeta struct {
	endpoint string
	retries  int
}

// RenamedProvider exercises the bridge mapping for every shape it needs to
// handle: scalar inputs/outputs and nested-block inputs/outputs on each of a
// resource, a data source, and provider config — all with non-default Pulumi
// names. Tests apply explicit renames via tfbridge.SchemaInfo so the engine
// must consult the bridge mapping to translate names in both directions.
//
// TF / Pulumi name correspondences (set up by callers' Customize hooks):
//
//   - provider config
//     `endpoint`                                  → `host`
//     `default_options { retries }`               → `defaults.retryCount`
//   - resource renamed_thing
//     `function_name`                             → `name`
//     `settings { window_size }`                  → `config.sizeWindow`
//   - data source renamed_lookup
//     `filter { kind }`                           → `lookupFilter.kindLabel`
//     `upstream`                                  → `source`
//     `result { tag }`                            → `outcome.label`
//
// The SDK schema below uses TF names directly so the upstream provider
// continues to validate and operate on them — only the Pulumi-side facing
// names change.
func RenamedProvider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"endpoint": {Type: schema.TypeString, Optional: true},
			"default_options": {
				Type:     schema.TypeList,
				Optional: true,
				MaxItems: 1,
				Elem: &schema.Resource{
					Schema: map[string]*schema.Schema{
						"retries": {Type: schema.TypeInt, Optional: true, Default: 0},
					},
				},
			},
		},
		ConfigureContextFunc: func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
			ep, _ := d.Get("endpoint").(string)
			meta := &renamedProviderMeta{endpoint: ep}
			if list, ok := d.Get("default_options").([]any); ok && len(list) > 0 {
				if m, ok := list[0].(map[string]any); ok {
					if r, ok := m["retries"].(int); ok {
						meta.retries = r
					}
				}
			}
			return meta, nil
		},
		ResourcesMap: map[string]*schema.Resource{
			"renamed_thing": {
				Schema: map[string]*schema.Schema{
					"function_name": {Type: schema.TypeString, Required: true},
					"settings": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: 1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"window_size": {Type: schema.TypeInt, Required: true},
							},
						},
					},
					// Computed outputs reflect provider config so the test can
					// assert provider input renames worked end-to-end.
					"arn":               {Type: schema.TypeString, Computed: true},
					"provider_endpoint": {Type: schema.TypeString, Computed: true},
					"provider_retries":  {Type: schema.TypeInt, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
					d.SetId("renamed-id")
					fn, _ := d.Get("function_name").(string)
					if err := d.Set("arn", "arn:test:"+fn); err != nil {
						return diag.FromErr(err)
					}
					if m, ok := meta.(*renamedProviderMeta); ok && m != nil {
						if err := d.Set("provider_endpoint", m.endpoint); err != nil {
							return diag.FromErr(err)
						}
						if err := d.Set("provider_retries", m.retries); err != nil {
							return diag.FromErr(err)
						}
					}
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"renamed_lookup": {
				Schema: map[string]*schema.Schema{
					"query": {Type: schema.TypeString, Required: true},
					"filter": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: 1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"kind": {Type: schema.TypeString, Required: true},
							},
						},
					},
					"matched":  {Type: schema.TypeString, Computed: true},
					"upstream": {Type: schema.TypeString, Computed: true},
					"result": {
						Type:     schema.TypeList,
						Computed: true,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"tag": {Type: schema.TypeString, Computed: true},
							},
						},
					},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					q, _ := d.Get("query").(string)
					d.SetId(q)
					kind := ""
					if list, ok := d.Get("filter").([]any); ok && len(list) > 0 {
						if m, ok := list[0].(map[string]any); ok {
							kind, _ = m["kind"].(string)
						}
					}
					if err := d.Set("matched", "hit:"+q+":"+kind); err != nil {
						return diag.FromErr(err)
					}
					if err := d.Set("upstream", "registry"); err != nil {
						return diag.FromErr(err)
					}
					return diag.FromErr(d.Set("result", []map[string]any{{"tag": "tag-for-" + q}}))
				},
			},
		},
	}
}
