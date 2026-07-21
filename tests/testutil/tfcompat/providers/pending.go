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
	"sync"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PendingProvider models a resource that is created and then read back through
// a data source: `pending_thing` registers its name in per-provider state at
// Create, and `pending_lookup` errors unless that name is already registered.
// A read issued before the create is therefore observable as a failure rather
// than as an extra recorded op.
func PendingProvider() *schema.Provider {
	var mu sync.Mutex
	created := map[string]bool{}

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"pending_thing": {
				Schema: map[string]*schema.Schema{
					"name": {Type: schema.TypeString, Required: true, ForceNew: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					name, _ := d.Get("name").(string)
					mu.Lock()
					created[name] = true
					mu.Unlock()
					d.SetId(name)
					return nil
				},
				ReadContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					mu.Lock()
					delete(created, d.Id())
					mu.Unlock()
					return nil
				},
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"pending_lookup": {
				Schema: map[string]*schema.Schema{
					"name":   {Type: schema.TypeString, Required: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					name, _ := d.Get("name").(string)
					mu.Lock()
					ok := created[name]
					mu.Unlock()
					if !ok {
						return diag.Errorf("pending_lookup: no pending_thing named %q exists", name)
					}
					d.SetId(name)
					return diag.FromErr(d.Set("result", "found-"+name))
				},
			},
		},
	}
}
