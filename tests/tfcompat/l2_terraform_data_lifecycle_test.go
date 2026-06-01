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

package tfcompat_test

import (
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// TestL2TerraformDataLifecycle drives terraform_data across its full lifecycle —
// create, an in-place update (input changes, id stable), and a replacement
// (triggers_replace changes, id rolls) — over three stages on one stack.
//
// terraform_data's id is a random uuid that differs between the tofu and pulumi
// paths, and its builtin provider records no CRUD ops, so the id can't be
// compared directly. Instead simple_resource.dependent's replace_triggered_by
// watches terraform_data.t.id: the dependent is replaced exactly when the id
// changes. Both paths must therefore produce the same simple-provider op trace —
// a create, no replacement on the input-only change, and a replacement on the
// triggers_replace change.
func TestL2TerraformDataLifecycle(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_terraform_data_lifecycle", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
