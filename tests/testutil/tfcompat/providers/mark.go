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

// MarkProvider proves operation ordering with an in-memory mark. Creating
// `mark_resource` flips the mark; the `mark_probe` data source reports the
// mark's current value via its computed `constructed` attribute. The mark is a
// fresh per-provider-instance closure variable, so the concurrently-running TF
// and Pulumi paths each observe only their own operations.
//
// `mark_probe` requires a `token`; passing `mark_resource`'s computed token
// makes the probe's read defer until the resource is created (the token is
// unknown until apply), so a check reading `mark_probe.constructed` observes
// true only when the check runs after the resource is constructed — exactly the
// ordering a scoped data source must obey.
func MarkProvider() *schema.Provider {
	var constructed atomic.Bool
	noop := func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil }
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"mark_resource": {
				Schema: map[string]*schema.Schema{
					// token is computed during Create, so a reference to it is
					// unknown until apply. A scoped data source that consumes it
					// must therefore defer its read until after this resource is
					// created on both runtimes.
					"token": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					constructed.Store(true)
					d.SetId("mark")
					return diag.FromErr(d.Set("token", "constructed"))
				},
				ReadContext:   noop,
				DeleteContext: noop,
			},
		},
		DataSourcesMap: map[string]*schema.Resource{
			"mark_probe": {
				Schema: map[string]*schema.Schema{
					"token":       {Type: schema.TypeString, Required: true},
					"constructed": {Type: schema.TypeBool, Computed: true},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("probe")
					if err := d.Set("constructed", constructed.Load()); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
			},
		},
	}
}
