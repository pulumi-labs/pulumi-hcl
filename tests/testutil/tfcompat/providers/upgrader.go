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

	"github.com/hashicorp/go-cty/cty"
	"github.com/hashicorp/terraform-plugin-sdk/v2/diag"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
)

// UpgraderProvider exposes a resource at schema version 2 whose state
// upgraders rewrite `note`, and whose read cannot recover it — the API returns
// a placeholder, as it does for a value a provider only ever writes.
//
// Both properties are load-bearing for the state import. The read means an
// import that carries no values cannot recover `note`, and the upgraders mean
// an import that carries values but loses the state's schema version has them
// rewritten on the first operation, since state at an unrecorded version reads
// as version 0 and runs the whole upgrade chain.
func UpgraderProvider() *schema.Provider {
	upgrade := func(to string) schema.StateUpgradeFunc {
		return func(_ context.Context, state map[string]any, _ any) (map[string]any, error) {
			state["note"] = to
			return state, nil
		}
	}
	priorType := cty.Object(map[string]cty.Type{"id": cty.String, "note": cty.String})
	return &schema.Provider{
		ResourcesMap: map[string]*schema.Resource{
			"upgrader_resource": {
				SchemaVersion: 2,
				StateUpgraders: []schema.StateUpgrader{
					{Version: 0, Type: priorType, Upgrade: upgrade("upgraded-from-v0")},
					{Version: 1, Type: priorType, Upgrade: upgrade("upgraded-from-v1")},
				},
				Schema: map[string]*schema.Schema{
					"note": {Type: schema.TypeString, Required: true},
				},
				Importer: &schema.ResourceImporter{
					StateContext: schema.ImportStatePassthroughContext,
				},
				CreateContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					d.SetId("only")
					return nil
				},
				ReadContext: func(_ context.Context, d *schema.ResourceData, _ any) diag.Diagnostics {
					return diag.FromErr(d.Set("note", "unrecoverable-by-read"))
				},
				UpdateContext: func(_ context.Context, _ *schema.ResourceData, _ any) diag.Diagnostics {
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
