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

	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// VersionedProvider exposes a resource that declares a non-zero schema
// version and supports no import, like tls_private_key (version 1) or
// aws_instance (version 2). Its state instances carry that same version, so an
// importer that only trusts version-0 state has to fall back to importing by
// id — which this resource cannot do.
func VersionedProvider() *schema.Provider {
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"versioned_resource": {
				SchemaVersion: 1,
				Schema: map[string]*schema.Schema{
					"algorithm": {Type: schema.TypeString, Required: true, ForceNew: true},
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("key-" + d.Get("algorithm").(string))
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
