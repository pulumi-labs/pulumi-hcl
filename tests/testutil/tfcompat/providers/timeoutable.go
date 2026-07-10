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

// TimeoutableProvider exposes a resource that declares operation timeouts, so
// HCL programs can set a `timeouts` block on it.
func TimeoutableProvider() *schema.Provider {
	dur := 20 * time.Minute
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"timeoutable_resource": {
				Timeouts: &schema.ResourceTimeout{
					Create: &dur,
					Update: &dur,
					Delete: &dur,
				},
				Schema: map[string]*schema.Schema{
					"input_one": {Type: schema.TypeString, Optional: true},
					"result":    {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("timeoutable-id")
					in, _ := d.Get("input_one").(string)
					return diag.FromErr(d.Set("result", in+"-done"))
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}
