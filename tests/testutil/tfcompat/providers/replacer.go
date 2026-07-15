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

// ReplacerProvider exposes `replacer_resource`, a resource that can be forced
// to be replaced. Its `force` attribute is ForceNew, so changing it destroys
// and recreates the resource. `note` is an ordinary in-place-updatable input.
// `result` is a computed value recomputed on both create and update, so the
// resource's referenced value tracks its inputs (unlike SimpleProvider, whose
// no-op Update leaves computed fields stale). The id is a constant, so a
// replacement leaves the resource's id unchanged.
func ReplacerProvider() *schema.Provider {
	compute := func(d *schema.ResourceData) diag.Diagnostics {
		force, _ := d.Get("force").(string)
		note, _ := d.Get("note").(string)
		return diag.FromErr(d.Set("result", force+"/"+note))
	}
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"replacer_resource": {
				Schema: map[string]*schema.Schema{
					"force":  {Type: schema.TypeString, Optional: true, ForceNew: true},
					"note":   {Type: schema.TypeString, Optional: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("replacer-id")
					return compute(d)
				},
				ReadContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					return compute(d)
				},
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}
