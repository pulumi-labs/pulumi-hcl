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

// DestroyOrderingProvider exposes `destroyorder_resource`, whose Delete
// holds for a few seconds and fails if another Delete is in flight at the same
// time. Create is cheap and never asserts, so apply always succeeds; only the
// destroy ordering is observed. Chaining two of these so that the dependent
// references the dependency asserts serialized destruction: when the ordering
// is honored the two Deletes never overlap; when the dependency edge is missing
// the two Deletes start concurrently and both error.
//
// The in-flight counter is captured per provider instance (one factory call per
// runtime), so a runtime's own resources share it while the concurrently-run
// Terraform and pulumi runtimes keep separate counters.
func DestroyOrderingProvider() *schema.Provider {
	var mu sync.Mutex
	inFlight := 0

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"destroyorder_resource": {
				Schema: map[string]*schema.Schema{
					"name": {Type: schema.TypeString, Optional: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("destroy-ordering-id")
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
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
							"destroyorder_resource %q deleted while another delete was still in "+
								"flight: dependency destroy ordering was not honored", name)
					}

					// Hold the slot open long enough that an unordered second
					// delete reliably starts within this window.
					time.Sleep(3 * time.Second)
					return nil
				},
			},
		},
	}
}
