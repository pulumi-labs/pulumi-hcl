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
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// OrderingProvider exposes `ordering_resource`, whose Create holds for a few
// seconds and fails if another Create is in flight at the same time. Chaining
// two of these with depends_on asserts serialized creation: when the ordering
// is honored the two Creates never overlap; when it is not, the second Create
// starts during the first's window and both error.
//
// The in-flight counter is captured per provider instance (one factory call per
// runtime), so a runtime's own resources share it while the concurrently-run
// Terraform and pulumi runtimes keep separate counters.
func OrderingProvider() *schema.Provider {
	var mu sync.Mutex
	inFlight := 0

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"ordering_resource": {
				Schema: map[string]*schema.Schema{
					"name": {Type: schema.TypeString, Optional: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					mu.Lock()
					inFlight++
					overlapped := inFlight > 1
					mu.Unlock()
					defer func() {
						mu.Lock()
						inFlight--
						mu.Unlock()
					}()

					if overlapped {
						name, _ := d.Get("name").(string)
						return diag.Errorf(
							"ordering_resource %q created while another create was still in flight: "+
								"depends_on ordering was not honored", name)
					}

					// Hold the slot open long enough that an unordered second
					// create reliably starts within this window.
					time.Sleep(3 * time.Second)

					d.SetId("ordering-id")
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}

// OrderDepProvider exposes `orderdep_resource`, which enforces destroy
// ordering. Each resource has a `name` and an optional `needs` (the name of the
// resource it depends on). A resource that declares `needs` sleeps during its
// own Delete and then asserts its dependency has not been deleted yet, so the
// dependency must be destroyed *after* the dependent; if it is destroyed first,
// the dependent's Delete fails.
//
// The `deleted` set is captured per factory call (one per runtime), so the
// concurrently-run Terraform and pulumi runtimes keep independent state.
func OrderDepProvider() *schema.Provider {
	var mu sync.Mutex
	deleted := map[string]bool{}

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"orderdep_resource": {
				Schema: map[string]*schema.Schema{
					"name":   {Type: schema.TypeString, Required: true},
					"needs":  {Type: schema.TypeString, Optional: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					name, _ := d.Get("name").(string)
					d.SetId(name)
					if err := d.Set("result", name); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					name, _ := d.Get("name").(string)
					needs, _ := d.Get("needs").(string)
					if needs != "" {
						// Give an out-of-order delete of `needs` time to land.
						time.Sleep(1 * time.Second)
						mu.Lock()
						already := deleted[needs]
						mu.Unlock()
						if already {
							return diag.Errorf(
								"orderdep_resource %q: its dependency %q was destroyed first; "+
									"destroy order was not honored", name, needs)
						}
					}
					mu.Lock()
					deleted[name] = true
					mu.Unlock()
					return nil
				},
			},
		},
	}
}
