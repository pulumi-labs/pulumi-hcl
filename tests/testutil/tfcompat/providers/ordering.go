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
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// OrderProvider exposes `order_resource` and the `order_data` data source,
// minimal objects for Case.OrderDeterministic ordering tests: the recorded op
// sequence itself asserts ordering, so they carry no assertion logic of their
// own.
//
// `delay_create`/`delay_delete`/`delay_read` hold that operation open briefly.
// Set the delay on the op that must complete *first* in the correct order.
// When the dependency edge under test is honored the delay only stretches the
// already serialized sequence; when the edge is missing the two ops run
// concurrently and the undelayed op reliably records ahead of the delayed one,
// so the regression flips the recorded order deterministically instead of
// leaving the comparison to a race.
func OrderProvider() *schema.Provider {
	const delay = 1 * time.Second
	noop := func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil }
	return &schema.Provider{
		DataSourcesMap: map[string]*schema.Resource{
			"order_data": {
				Schema: map[string]*schema.Schema{
					"name":       {Type: schema.TypeString, Required: true},
					"delay_read": {Type: schema.TypeBool, Optional: true},
					"result":     {Type: schema.TypeString, Computed: true},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					if b, _ := d.Get("delay_read").(bool); b {
						time.Sleep(delay)
					}
					name, _ := d.Get("name").(string)
					d.SetId(name)
					return diag.FromErr(d.Set("result", name))
				},
			},
		},
		ResourcesMap: map[string]*schema.Resource{
			"order_resource": {
				Schema: map[string]*schema.Schema{
					"name":         {Type: schema.TypeString, Required: true},
					"delay_create": {Type: schema.TypeBool, Optional: true},
					"delay_delete": {Type: schema.TypeBool, Optional: true},
					"result":       {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					if b, _ := d.Get("delay_create").(bool); b {
						time.Sleep(delay)
					}
					name, _ := d.Get("name").(string)
					d.SetId(name)
					return diag.FromErr(d.Set("result", name))
				},
				ReadContext:   noop,
				UpdateContext: noop,
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					if b, _ := d.Get("delay_delete").(bool); b {
						time.Sleep(delay)
					}
					return nil
				},
			},
		},
	}
}
