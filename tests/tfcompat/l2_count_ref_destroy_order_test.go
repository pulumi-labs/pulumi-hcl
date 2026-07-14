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

package tfcompat_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
)

// orderDepProvider exposes `orderdep_resource`, which enforces destroy
// ordering. Each resource has a `name` and an optional `needs` (the name of
// the resource it depends on). A resource that declares `needs` sleeps during
// its own Delete and then asserts its dependency has not been deleted yet. So
// the dependency must be destroyed *after* the dependent; if it is destroyed
// first, the dependent's Delete fails.
//
// The `deleted` set is captured per factory call (one per runtime), so the
// concurrently-run tofu and pulumi runtimes keep independent state, mirroring
// providers.OrderingProvider.
func orderDepProvider() *schema.Provider {
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
						time.Sleep(3 * time.Second)
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

// TestL2CountRefDestroyOrder proves that a reference expressed only through a
// resource's `count` meta-argument establishes a dependency that governs
// destroy ordering. `b`'s `count` references `a.result`; nothing in `b`'s body
// references `a`. On apply both runtimes create `a` before `b` (the count needs
// `a.result`). On destroy, OpenTofu honors the count-derived dependency and
// destroys `b` before `a`; pulumi-hcl derives dependencies only from input
// values and misses the edge, destroying `a` first and failing `b`'s Delete.
func TestL2CountRefDestroyOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_count_ref_destroy_order", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "orderdep", Factory: orderDepProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StageApply},
			{Mode: tfcompat.StageDestroy},
		},
	})
}
