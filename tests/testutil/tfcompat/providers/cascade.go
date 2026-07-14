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

// CascadeProvider exposes `cascade_parent` and `cascade_child`. The child wires
// its ForceNew `parent` input to the parent's `result`, so replacing the parent
// (by changing its ForceNew `label`) replaces the child too.
//
// Every Create and Delete bumps a per-provider operation counter, and the child
// records the counter value at its own Create in the computed `witness`. This
// makes the relative order of the replacement's creates and deletes observable
// as a value.
func CascadeProvider() *schema.Provider {
	var mu sync.Mutex
	opSeq := 0
	next := func() int {
		mu.Lock()
		defer mu.Unlock()
		opSeq++
		return opSeq
	}

	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"cascade_parent": {
				Schema: map[string]*schema.Schema{
					"label":  {Type: schema.TypeString, Required: true, ForceNew: true},
					"result": {Type: schema.TypeString, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					next()
					label, _ := d.Get("label").(string)
					d.SetId(label)
					return diag.FromErr(d.Set("result", label))
				},
				ReadContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics {
					next()
					return nil
				},
			},
			"cascade_child": {
				Schema: map[string]*schema.Schema{
					"parent":  {Type: schema.TypeString, Required: true, ForceNew: true},
					"witness": {Type: schema.TypeInt, Computed: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					seq := next()
					parent, _ := d.Get("parent").(string)
					d.SetId("child-" + parent)
					return diag.FromErr(d.Set("witness", seq))
				},
				ReadContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics { return nil },
				DeleteContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics {
					next()
					return nil
				},
			},
		},
	}
}
