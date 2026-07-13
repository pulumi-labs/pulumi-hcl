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
	"sync/atomic"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// PartialDestroyProvider exposes `partial_destroy_resource`. A resource with
// fail_delete=true errors the first time it is deleted, then succeeds. Ordering
// a normal resource to be deleted before such a resource makes the first
// destroy abort with the normal resource already gone from state — the
// partial-destroy condition under which a --run-program destroy re-runs the
// program and evaluates an output indexing the now-absent resource.
//
// The one-shot failure is captured per provider instance (one factory call per
// runtime), so the concurrently-run Terraform and pulumi runtimes fail
// independently.
func PartialDestroyProvider() *schema.Provider {
	var failedOnce atomic.Bool

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"partialdestroy_resource": {
				Schema: map[string]*schema.Schema{
					"zones": {
						Type:     schema.TypeList,
						Optional: true,
						Elem:     &schema.Schema{Type: schema.TypeString},
					},
					"fail_delete": {Type: schema.TypeBool, Optional: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("partial-destroy-id")
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					if fail, _ := d.Get("fail_delete").(bool); fail && !failedOnce.Swap(true) {
						return diag.Errorf("delete intentionally failed on first attempt")
					}
					return nil
				},
			},
		},
	}
}
