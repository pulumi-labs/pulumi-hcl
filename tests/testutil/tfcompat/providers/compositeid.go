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
	"errors"

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// CompositeIDProvider mirrors resources like aws_iam_role_policy_attachment,
// whose importer wants a composite "role/policy" string rather than the
// resource's own opaque id. Its importer is a tripwire that fails on any
// call, so a passing round-trip proves a values-supplied import never
// invokes it and so never needs the composite form.
func CompositeIDProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"composite_attachment": {
				Schema: map[string]*schema.Schema{
					"role":   {Type: schema.TypeString, Required: true, ForceNew: true},
					"policy": {Type: schema.TypeString, Required: true, ForceNew: true},
				},
				Importer: &schema.ResourceImporter{
					StateContext: func(_ context.Context, _ *schema.ResourceData, _ any) ([]*schema.ResourceData, error) {
						return nil, errors.New("unexpected call to import")
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
