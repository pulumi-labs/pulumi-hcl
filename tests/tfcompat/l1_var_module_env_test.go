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
)

// TestL1VarModuleEnv pins that TF_VAR_<name> sets root variables only: a module
// variable with no input takes its default, whatever the environment says.
//
// Sets an environment variable, so it must not run in parallel.
func TestL1VarModuleEnv(t *testing.T) {
	t.Setenv("TF_VAR_who", "from-env")
	tfcompat.RunCase(t, "l1_var_module_env", tfcompat.Case{})
}
