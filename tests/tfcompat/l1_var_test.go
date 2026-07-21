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
	"github.com/stretchr/testify/assert"
)

// A root variable's `default` whose literal element/attribute types differ from
// the declared `type` (e.g. list(string) default written as [1, true]) must be
// coerced to the type constraint. Stack outputs must match.
func TestL1Var(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var", tfcompat.Case{})
}

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

// TestL1VarTfvarsFile pins the automatic loading of `terraform.tfvars` from
// the program directory: it takes precedence over the variable's default.
func TestL1VarTfvarsFile(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_file", tfcompat.Case{})
}

// TestL1VarTfvarsOrder pins the order the automatically-loaded variable-value
// files are applied in, including the lexical interleave of `*.auto.tfvars`
// and `*.auto.tfvars.json`.
func TestL1VarTfvarsOrder(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_order", tfcompat.Case{})
}

// TestL1VarTfvarsConvert pins that a value that does not fit the variable's
// declared type is an error, not a value that survives unconverted.
func TestL1VarTfvarsConvert(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_convert", tfcompat.Case{
		Stages: []tfcompat.Stage{{ExpectErr: "given value is not suitable for var.n"}},
	})
}

// TestL1VarDefaultConvert pins that a `default` that does not fit the declared
// type is an error, not a value that survives unconverted.
func TestL1VarDefaultConvert(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_default_convert", tfcompat.Case{
		Stages: []tfcompat.Stage{{ExpectErr: "not compatible with the variable's type constraint"}},
	})
}

// TestL1VarTfvarsUndeclared pins that a variable-value file setting a name the
// root module does not declare warns and runs: the value reaches nothing, and
// its expression is never evaluated.
func TestL1VarTfvarsUndeclared(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_undeclared", tfcompat.Case{
		Stages: []tfcompat.Stage{{
			AssertOutput: func(t *testing.T, output string) {
				assert.Contains(t, output, "Value for undeclared variable")
			},
		}},
	})
}

// TestL1VarTfvarsDir pins that a `terraform.tfvars` that is a directory is a
// variable-value file that cannot be read, not an absent one.
func TestL1VarTfvarsDir(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_dir", tfcompat.Case{
		Stages: []tfcompat.Stage{{ExpectErr: "reading terraform.tfvars"}},
	})
}

// TestL1VarTfvarsModule pins that variable-value files are loaded for the root
// module only: a module ships its own `terraform.tfvars` and it is inert.
func TestL1VarTfvarsModule(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l1_var_tfvars_module", tfcompat.Case{})
}

// TestL1VarModuleEnv pins that TF_VAR_<name> sets root variables only: a module
// variable with no input takes its default, whatever the environment says.
//
// Sets an environment variable, so it must not run in parallel.
func TestL1VarModuleEnv(t *testing.T) {
	t.Setenv("TF_VAR_who", "from-env")
	tfcompat.RunCase(t, "l1_var_module_env", tfcompat.Case{})
}
