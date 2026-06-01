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

func TestL2Provisioner_LocalExecPass(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_local_exec_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"
  input_two = false

  provisioner "local-exec" {
    command = "true"
  }
}
`},
		}},
	})
}

func TestL2Provisioner_LocalExecFail(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_local_exec_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command = "exit 1"
  }
}
`},
			// "exit status 1" appears in both runtimes' error output.
			ExpectErr: "exit status 1",
		}},
	})
}

func TestL2Provisioner_OnFailureContinue(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_on_failure_continue", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command    = "exit 1"
    on_failure = "continue"
  }
}
`},
		}},
	})
}

func TestL2Provisioner_SelfReference(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_self", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"
  input_two = true

  provisioner "local-exec" {
    command = "test '${self.result}' = 'a-true'"
  }
}
`},
		}},
	})
}

// self.id reference proves prior state is available during destroy.
func TestL2Provisioner_WhenDestroyPass(t *testing.T) {
	t.Parallel()
	program := map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "test -n '${self.id}'"
  }
}
`}
	tfcompat.RunCase(t, "l2_provisioner_when_destroy_pass", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Files: program},
			{Files: program, Mode: tfcompat.StageDestroy},
		},
	})
}

func TestL2Provisioner_WhenDestroyFail(t *testing.T) {
	t.Parallel()
	program := map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    when    = destroy
    command = "exit 1"
  }
}
`}
	tfcompat.RunCase(t, "l2_provisioner_when_destroy_fail", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{
			{Files: program},
			{Files: program, Mode: tfcompat.StageDestroy, ExpectErr: "exit status 1"},
		},
	})
}

// The harness can't observe stdout suppression directly; if quiet broke
// command invocation either runtime would fail.
func TestL2Provisioner_Quiet(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_quiet", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command = "echo this-should-be-suppressed"
    quiet   = true
  }
}
`},
		}},
	})
}

// Failing command paired with preview-mode: success proves the
// provisioner didn't run.
func TestL2Provisioner_NotInPreview(t *testing.T) {
	t.Parallel()
	tfcompat.RunCase(t, "l2_provisioner_not_in_preview", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{
			Files: map[string]string{"main.tf": `
resource "simple_resource" "target" {
  input_one = "a"

  provisioner "local-exec" {
    command = "exit 1"
  }
}
`},
			Mode: tfcompat.StagePreview,
		}},
	})
}

