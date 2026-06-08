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

func TestL2NestedComputedBlock(t *testing.T) {
	t.Parallel()
	main := `resource "nested_cluster" "this" {
}

output "issuer" {
  value = nested_cluster.this.identity[0].oidc[0].issuer
}
`
	tfcompat.RunCase(t, "l2_nested_computed_block", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "nested", Factory: providers.NestedProvider},
		},
		Stages: []tfcompat.Stage{
			{Files: map[string]string{"main.tf": main}, Mode: tfcompat.StagePreview},
		},
	})
}
