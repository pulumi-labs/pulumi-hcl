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

// TestL2ModuleUnknownInputOptionalAttr drives a module input that is unknown at
// preview and whose declared type adds an optional attribute the source object
// lacks. A module argument must be retyped to the variable's declared type even
// when the value is unknown, so the module body sees the declared object type
// (with the optional `c`) and `var.cfg.c` is valid at preview.
func TestL2ModuleUnknownInputOptionalAttr(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_module_unknown_input_optional_attr", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Mode: tfcompat.StagePreview}},
	})
}
