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

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// TypeClashProvider has two resources whose nested object types the bridge
// names identically: `typeclash_res.block[].nested_block[]` and
// `typeclash_res_block.nested_block[]` both become `ResBlockNestedBlock`. The
// two declarations disagree on their `inner_block` MaxItems, so the single
// Pulumi type the bridge keeps for the name does not match one resource's TF
// shape.
func TypeClashProvider() *schema.Provider {
	// nestedBlock is the shared-named block; its inner_block is MaxItems=1
	// under typeclash_res and unbounded under typeclash_res_block.
	nestedBlock := func(innerMaxItems int) *schema.Schema {
		return &schema.Schema{
			Type:     schema.TypeList,
			Optional: true,
			Elem: &schema.Resource{
				Schema: map[string]*schema.Schema{
					"inner_block": {
						Type:     schema.TypeList,
						Optional: true,
						MaxItems: innerMaxItems,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"attr": {Type: schema.TypeString, Required: true},
							},
						},
					},
				},
			},
		}
	}
	summarize := func(d *schema.ResourceData, path string) diag.Diagnostics {
		d.SetId(path)
		return diag.FromErr(d.Set("summary", fmt.Sprintf("%v", d.Get(path))))
	}
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"typeclash_res": {
				Schema: map[string]*schema.Schema{
					"block": {
						Type:     schema.TypeList,
						Optional: true,
						Elem: &schema.Resource{
							Schema: map[string]*schema.Schema{
								"name":         {Type: schema.TypeString, Required: true},
								"nested_block": nestedBlock(1),
							},
						},
					},
					"summary": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					return summarize(d, "block")
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
			"typeclash_res_block": {
				Schema: map[string]*schema.Schema{
					"nested_block": nestedBlock(0),
					"summary":      {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					return summarize(d, "nested_block")
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}
