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
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/sshd"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// Omits host_key so the SSH communicator falls back to
// InsecureIgnoreHostKey — fine for ephemeral test containers.
func sshConnectionHCL(c *sshd.Container) string {
	return fmt.Sprintf(`connection {
    type     = "ssh"
    host     = %q
    port     = %d
    user     = %q
    password = %q
    timeout  = "30s"
  }`, c.Host, c.Port, c.User, c.Password)
}

func TestL2Provisioner_RemoteExecInline(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "remote-exec" {
    inline = ["echo hello-from-remote-exec"]
  }
}
`, sshConnectionHCL(c))}

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_inline", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}

func TestL2Provisioner_RemoteExecScript(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "hello.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\necho script-ran\n"), 0o755))

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "remote-exec" {
    script = %q
  }
}
`, sshConnectionHCL(c), scriptPath)}

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_script", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}

func TestL2Provisioner_RemoteExecScripts(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	scriptDir := t.TempDir()
	first := filepath.Join(scriptDir, "first.sh")
	second := filepath.Join(scriptDir, "second.sh")
	require.NoError(t, os.WriteFile(first, []byte("#!/bin/sh\necho first\n"), 0o755))
	require.NoError(t, os.WriteFile(second, []byte("#!/bin/sh\necho second\n"), 0o755))

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "remote-exec" {
    scripts = [%q, %q]
  }
}
`, sshConnectionHCL(c), first, second)}

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_scripts", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}

// Default script_path is /tmp/terraform_%RAND%.sh, which isn't writable by
// the test user. Pointing it at /config (the linuxserver home) means
// success proves the override took effect.
func TestL2Provisioner_ScriptPath(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "hello.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\necho custom-script-path-ok\n"), 0o755))

	conn := fmt.Sprintf(`connection {
    type        = "ssh"
    host        = %q
    port        = %d
    user        = %q
    password    = %q
    timeout     = "30s"
    script_path = "/config/terraform_%%RAND%%.sh"
  }`, c.Host, c.Port, c.User, c.Password)

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "remote-exec" {
    script = %q
  }
}
`, conn, scriptPath)}

	tfcompat.RunCase(t, "l2_provisioner_script_path", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}

func TestL2Provisioner_FileFromContent(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "file" {
    content     = "hello-from-file-provisioner\n"
    destination = "/config/from-content.txt"
  }
}
`, sshConnectionHCL(c))}

	tfcompat.RunCase(t, "l2_provisioner_file_content", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}

func TestL2Provisioner_FileFromSource(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "upload.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello-from-source\n"), 0o644))

	program := map[string]string{"main.tf": fmt.Sprintf(`
resource "simple_resource" "target" {
  input_one = "a"

  %s

  provisioner "file" {
    source      = %q
    destination = "/config/from-source.txt"
  }
}
`, sshConnectionHCL(c), src)}

	tfcompat.RunCase(t, "l2_provisioner_file_source", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Stages: []tfcompat.Stage{{Files: program}},
	})
}
