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

// SingleSegmentProvider mirrors the shape of real providers like
// `hashicorp/external` and `hashicorp/http`, where the type name is the same
// single word as the provider — so HCL writes `data "single" "x"` and
// `resource "single" "y"` with no underscore in the type token.
func SingleSegmentProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"single": {
				Schema: map[string]*schema.Schema{
					"input":  {Type: schema.TypeString, Optional: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("single-id")
					in, _ := d.Get("input").(string)
					return diag.FromErr(d.Set("result", "r-"+in))
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"single": {
				Schema: map[string]*schema.Schema{
					"query":  {Type: schema.TypeString, Required: true},
					"answer": {Type: schema.TypeString, Computed: true},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					q, _ := d.Get("query").(string)
					d.SetId("q-" + q)
					return diag.FromErr(d.Set("answer", "a-"+q))
				},
			},
		},
	}
}
