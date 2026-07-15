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

// ImportableProvider exposes a resource that supports config-driven `import`
// blocks. Its Read derives the `name` attribute from the resource id, so a
// test can observe which id each instance was imported with (the Read is
// recorded, and `name` is exposed as an output).
func ImportableProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"importable_resource": {
				Schema: map[string]*schema.Schema{
					// Derived from the id on Read/Create, so the imported
					// identity is visible in state and outputs.
					"name": {Type: schema.TypeString, Computed: true},
				},
				Importer: &schema.ResourceImporter{
					StateContext: schema.ImportStatePassthroughContext,
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("created")
					if err := d.Set("name", "created-"+d.Id()); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					if err := d.Set("name", "imported-"+d.Id()); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("")
					return nil
				},
			},
		},
	}
}
