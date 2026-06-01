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

// A `version` argument on a resource block must reach the provider as a
// regular property value, not be reinterpreted as a Pulumi plugin-version
// override. Real-world example: `data "aws_rds_engine_version" "x" { version
// = "15.4" }` in the aws-ia/rds-aurora module.
func TestL2DataVersionAttribute(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_data_version_attribute", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
