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

// depends_on referencing a single module instance (`module.m["x"]`) is
// accepted by tofu, but the dependency applies to the whole module call:
// `b` waits for the delayed m["y"].a as well. Guards against over-narrowing
// instance-keyed module depends_on.
func TestL2ModuleInstanceDependsOn(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_instance_depends_on", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "order", Factory: providers.OrderProvider},
		},
		OrderDeterministic: true,
	})
}
