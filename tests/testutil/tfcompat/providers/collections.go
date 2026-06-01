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
	"fmt"
	"sort"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// CollectionsProvider exposes collection-shaped attributes that no other test
// provider exercises:
//
//   - `tags` is TypeMap of strings — the ubiquitous `tags = { ... }` shape
//     every real cloud resource has.
//   - `ports` is TypeSet of ints — an unordered primitive collection.
//   - `zones` is TypeList of strings — an ordered primitive collection.
//   - `rule` is TypeSet of blocks (Elem *Resource) — the unordered repeated
//     block shape of e.g. security-group ingress/egress rules.
//   - `metadata` is a singular block whose own attributes are a map and a set,
//     exercising primitive collections nested inside a block.
//
// Each value is folded deterministically into the `summary` output so both
// paths can be compared on what the provider actually received.
func CollectionsProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"collections_thing": {
				Schema: map[string]*schema.Schema{
					"tags": {
						Type:     schema.TypeMap,
						Optional: true,
						Elem:     &schema.Schema{Type: schema.TypeString},
					},
					"ports": {
						Type:     schema.TypeSet,
						Optional: true,
						Elem:     &schema.Schema{Type: schema.TypeInt},
					},
					"zones": {
						Type:     schema.TypeList,
						Optional: true,
						Elem:     &schema.Schema{Type: schema.TypeString},
					},
					"rule": {
						Type:     schema.TypeSet,
						Optional: true,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"port":     {Type: schema.TypeInt, Required: true},
								"protocol": {Type: schema.TypeString, Optional: true, Default: "tcp"},
							},
						},
					},
					"metadata": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: 1,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"labels": {
									Type:     schema.TypeMap,
									Optional: true,
									Elem:     &schema.Schema{Type: schema.TypeString},
								},
								"selectors": {
									Type:     schema.TypeSet,
									Optional: true,
									Elem:     &schema.Schema{Type: schema.TypeString},
								},
							},
						},
					},
					"summary": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("collections-id")
					var parts []string

					if m, ok := d.Get("tags").(map[string]any); ok && len(m) > 0 {
						parts = append(parts, "tags=["+joinSortedMap(m)+"]")
					}

					if set, ok := d.Get("ports").(*schema.Set); ok && set.Len() > 0 {
						var ports []int
						for _, p := range set.List() {
							if n, ok := p.(int); ok {
								ports = append(ports, n)
							}
						}
						sort.Ints(ports)
						strs := make([]string, len(ports))
						for i, p := range ports {
							strs[i] = fmt.Sprintf("%d", p)
						}
						parts = append(parts, "ports=["+strings.Join(strs, ",")+"]")
					}

					if list, ok := d.Get("zones").([]any); ok && len(list) > 0 {
						zones := make([]string, len(list))
						for i, z := range list {
							zones[i], _ = z.(string)
						}
						parts = append(parts, "zones=["+strings.Join(zones, ",")+"]")
					}

					if set, ok := d.Get("rule").(*schema.Set); ok && set.Len() > 0 {
						rules := make([]string, 0, set.Len())
						for _, r := range set.List() {
							m, _ := r.(map[string]any)
							port, _ := m["port"].(int)
							proto, _ := m["protocol"].(string)
							rules = append(rules, fmt.Sprintf("%s/%d", proto, port))
						}
						sort.Strings(rules)
						parts = append(parts, "rules=["+strings.Join(rules, ",")+"]")
					}

					if list, ok := d.Get("metadata").([]any); ok && len(list) > 0 {
						meta, _ := list[0].(map[string]any)
						if labels, ok := meta["labels"].(map[string]any); ok && len(labels) > 0 {
							parts = append(parts, "labels=["+joinSortedMap(labels)+"]")
						}
						if sel, ok := meta["selectors"].(*schema.Set); ok && sel.Len() > 0 {
							ss := make([]string, 0, sel.Len())
							for _, s := range sel.List() {
								ss = append(ss, fmt.Sprintf("%v", s))
							}
							sort.Strings(ss)
							parts = append(parts, "selectors=["+strings.Join(ss, ",")+"]")
						}
					}

					return diag.FromErr(d.Set("summary", strings.Join(parts, "|")))
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}

// joinSortedMap renders a string-valued map as `k=v` pairs sorted by key.
func joinSortedMap(m map[string]any) string {
	pairs := make([]string, 0, len(m))
	for k, v := range m {
		pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
	}
	sort.Strings(pairs)
	return strings.Join(pairs, ",")
}
