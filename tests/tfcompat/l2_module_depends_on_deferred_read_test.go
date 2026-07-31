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

// `depends_on` on a module call covers everything inside the module, so a data
// source in the child defers its read until the dependency is applied — even
// though the read's own arguments are known and no value flows from the
// resource into the module.
func TestL2ModuleDependsOnDeferredRead(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_depends_on_deferred_read", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "pending", Factory: providers.PendingProvider},
		},
		Stages: []tfcompat.Stage{
			{Mode: tfcompat.StagePreview},
			{},
		},
	})
}
