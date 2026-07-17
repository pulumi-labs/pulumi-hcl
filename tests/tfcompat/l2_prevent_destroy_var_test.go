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

// TestL2PreventDestroyVar proves that pulumi-hcl rejects a non-literal
// `prevent_destroy` expression that OpenTofu accepts.
//
// The resource sets `prevent_destroy = var.flag`. OpenTofu evaluates the
// expression and applies successfully (and, with flag=true, protects the
// resource exactly as a literal `prevent_destroy = true` would). pulumi-hcl
// decodes the `prevent_destroy` expression with a nil HCL eval context, so the
// `var.flag` reference fails to resolve and `pulumi up` aborts with "Variables
// not allowed" before the resource is created. The harness fails because
// pulumi errors on a program OpenTofu runs cleanly.
func TestL2PreventDestroyVar(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_prevent_destroy_var", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: map[string]string{"flag": "true"},
	})
}
