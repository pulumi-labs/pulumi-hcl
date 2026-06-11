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

	"github.com/hashicorp/go-cty/cty"
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
			// Any non-empty value fails ValidateDiagFunc. Lets tests assert
			// that schema validation on a `provider` block is only run when
			// something actually uses the provider (Terraform's lazy
			// configure behaviour).
			"fail_validate": {
				Type:     schema.TypeString,
				Optional: true,
				ValidateDiagFunc: func(v any, _ cty.Path) diag.Diagnostics {
					if s, _ := v.(string); s != "" {
						return diag.Errorf("simple provider: fail_validate was %q", s)
					}
					return nil
				},
			},
		},
		ConfigureContextFunc: func(_ context.Context, d *schema.ResourceData) (any, diag.Diagnostics) {
			prefix, _ := d.Get("prefix").(string)
			return &simpleProviderMeta{prefix: prefix}, nil
		},
		ResourcesMap: map[string]*schema.Resource{
			"simple_resource": {
				Schema: map[string]*schema.Schema{
					"input_one": {Type: schema.TypeString, Optional: true},
					"input_two": {Type: schema.TypeBool, Optional: true},
					// Changing this forces a replacement, so tests can stage
					// a resource with a pending replace.
					"input_replace": {Type: schema.TypeString, Optional: true, ForceNew: true},
					// `version` is an upstream-style attribute name used by
					// some real TF resources (e.g. aws_rds_engine_version).
					// Kept here so tests can assert it survives the
					// HCL→Pulumi translation and isn't stripped by any
					// meta-attribute reservation.
					"version":       {Type: schema.TypeString, Optional: true},
					"result":        {Type: schema.TypeString, Computed: true},
					"prefix_result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
					d.SetId("simple-id")
					one, _ := d.Get("input_one").(string)
					two, _ := d.Get("input_two").(bool)
					ver, _ := d.Get("version").(string)
					result := fmt.Sprintf("%s-%t", one, two)
					if ver != "" {
						result += "-" + ver
					}
					if err := d.Set("result", result); err != nil {
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
		DataSourcesMap: map[string]*schema.Resource{
			// `simple_lookup` echoes provider config (`prefix`) into
			// `prefix_result` so tests can assert that a `provider` block
			// referenced only by a data source — not by any resource — is
			// still configured.
			"simple_lookup": {
				Schema: map[string]*schema.Schema{
					"query":         {Type: schema.TypeString, Required: true},
					"prefix_result": {Type: schema.TypeString, Computed: true},
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, meta any) diag.Diagnostics {
					q, _ := d.Get("query").(string)
					var prefix string
					if m, ok := meta.(*simpleProviderMeta); ok && m != nil {
						prefix = m.prefix
					}
					d.SetId(prefix + "-" + q)
					if err := d.Set("prefix_result", prefix+"-"+q); err != nil {
						return diag.FromErr(err)
					}
					return nil
				},
			},
		},
	}
}
