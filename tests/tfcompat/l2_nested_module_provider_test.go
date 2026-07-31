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

	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// A nested module declares its own aliased provider block and a resource
// inside the module references it via `provider = simple.alpha`. The
// reference must resolve against the inner module's eval context.
// Reproduces the aws-ia/rds-aurora failure where `provider = aws.primary`
// in the aurora module was evaluated against the root context.
func TestL2NestedModuleProvider(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_nested_module_provider", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
