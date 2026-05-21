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

// Package providers holds reusable in-memory TF providers for tfcompat tests.
package providers

import (
	"context"
	"fmt"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

type simpleProviderMeta struct {
	prefix string
}

// SimpleProvider exposes one resource and a `prefix` provider-config
// attribute that the resource concatenates into `prefix_result`, so tests
// can observe provider config flowing end-to-end.
func SimpleProvider() *schema.Provider {
	return &schema.Provider{
		Schema: map[string]*schema.Schema{
			"prefix": {Type: schema.TypeString, Optional: true},
		},
		ConfigureContextFunc: func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
			prefix, _ := d.Get("prefix").(string)
			return &simpleProviderMeta{prefix: prefix}, nil
		},
		ResourcesMap: map[string]*schema.Resource{
			"simple_resource": {
				Schema: map[string]*schema.Schema{
					"input_one":     {Type: schema.TypeString, Optional: true},
					"input_two":     {Type: schema.TypeBool, Optional: true},
					"result":        {Type: schema.TypeString, Computed: true},
					"prefix_result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
					d.SetId("simple-id")
					one, _ := d.Get("input_one").(string)
					two, _ := d.Get("input_two").(bool)
					if err := d.Set("result", fmt.Sprintf("%s-%t", one, two)); err != nil {
						return diag.FromErr(err)
					}
					var prefix string
					if m, ok := meta.(*simpleProviderMeta); ok && m != nil {
						prefix = m.prefix
					}
					if err := d.Set("prefix_result", prefix+"-"+one); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
				ReadContext:   func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
			},
		},
	}
}
