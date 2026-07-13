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

// A resource-level provisioner / precondition / postcondition each registers a
// resource hook whose name must be unique per instance. When the resource lives
// in a module instantiated multiple times (for_each/count), a hook name derived
// from the in-module resource address alone collides across instances and
// pulumi-hcl fails with "resource hook already registered". Terraform has no
// such restriction, so these cases guard the fix that keys hook names by the
// per-instance Pulumi resource name.

func TestL2Module_ProvisionerForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_provisioner_for_each", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

func TestL2Module_PreconditionForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_precondition_for_each", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}

func TestL2Module_PostconditionForEach(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_postcondition_for_each", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
	})
}
