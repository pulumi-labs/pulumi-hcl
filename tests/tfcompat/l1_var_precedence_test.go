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

// TestL1VarPrecedence pins the order the sources of a root variable value are
// consulted in: an explicitly supplied value, then the variable-value files,
// then TF_VAR_<name>, then the declared default.
//
// Sets environment variables, so it must not run in parallel.
func TestL1VarPrecedence(t *testing.T) {
	t.Setenv("TF_VAR_config_beats_env", "from-env")
	t.Setenv("TF_VAR_tfvars_beats_env", "from-env")
	t.Setenv("TF_VAR_empty_env_beats_default", "")
	tfcompat.RunCase(t, "l1_var_precedence", tfcompat.Case{
		Config: map[string]string{
			"config_beats_tfvars": "from-config",
			"config_beats_env":    "from-config",
		},
	})
}
