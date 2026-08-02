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
	"fmt"
	"strings"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// CompositeIDProvider mirrors resources like aws_iam_role_policy_attachment:
// the importer parses a composite "role/policy" import string, while the
// resource's own id is an opaque internal value the importer would reject.
// The importer is a tripwire — a state import that reaches it fails loudly,
// so a passing round-trip proves values-supplied imports never invoke it.
func CompositeIDProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"composite_attachment": {
				Schema: map[string]*schema.Schema{
					"role":   {Type: schema.TypeString, Required: true, ForceNew: true},
					"policy": {Type: schema.TypeString, Required: true, ForceNew: true},
				},
				Importer: &schema.ResourceImporter{
					StateContext: func(_ context.Context, d *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
						parts := strings.SplitN(d.Id(), "/", 2)
						if len(parts) != 2 {
							return nil, fmt.Errorf("expected an import id in role/policy form, got %q", d.Id())
						}
						if err := d.Set("role", parts[0]); err != nil {
							return nil, err
						}
						if err := d.Set("policy", parts[1]); err != nil {
							return nil, err
						}
						d.SetId("internal:" + parts[0])
						return []*schema.ResourceData{d}, nil
					},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("internal:" + d.Get("role").(string))
					return nil
				},
				ReadContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics {
					return nil
				},
				DeleteContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("")
					return nil
				},
			},
		},
	}
}
