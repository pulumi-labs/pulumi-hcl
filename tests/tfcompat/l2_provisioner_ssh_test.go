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
	"maps"
	"os"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/stretchr/testify/require"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/sshd"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat/providers"
)

// The programs omit host_key so the SSH communicator falls back to
// InsecureIgnoreHostKey — fine for ephemeral test containers. The container's
// coordinates reach the program as variables.
func sshConfig(c *sshd.Container, extra map[string]string) map[string]string {
	cfg := map[string]string{
		"host":     c.Host,
		"port":     strconv.Itoa(c.Port),
		"user":     c.User,
		"password": c.Password,
	}
	maps.Copy(cfg, extra)
	return cfg
}

func TestL2Provisioner_RemoteExecInline(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_inline", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, nil),
	})
}

func TestL2Provisioner_RemoteExecScript(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "hello.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\necho script-ran\n"), 0o755))

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_script", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, map[string]string{"script": scriptPath}),
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

	tfcompat.RunCase(t, "l2_provisioner_remote_exec_scripts", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, map[string]string{
			"script_first":  first,
			"script_second": second,
		}),
	})
}

// Default script_path is /tmp/terraform_%RAND%.sh, which isn't writable by
// the test user. The program points it at /config (the linuxserver home), so
// success proves the override took effect.
func TestL2Provisioner_ScriptPath(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	scriptDir := t.TempDir()
	scriptPath := filepath.Join(scriptDir, "hello.sh")
	require.NoError(t, os.WriteFile(scriptPath, []byte("#!/bin/sh\necho custom-script-path-ok\n"), 0o755))

	tfcompat.RunCase(t, "l2_provisioner_script_path", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, map[string]string{"script": scriptPath}),
	})
}

func TestL2Provisioner_FileFromContent(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	tfcompat.RunCase(t, "l2_provisioner_file_content", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, nil),
	})
}

func TestL2Provisioner_FileFromSource(t *testing.T) {
	t.Parallel()
	c := sshd.Start(t.Context(), t)

	srcDir := t.TempDir()
	src := filepath.Join(srcDir, "upload.txt")
	require.NoError(t, os.WriteFile(src, []byte("hello-from-source\n"), 0o644))

	tfcompat.RunCase(t, "l2_provisioner_file_source", tfcompat.Case{
		Providers: []tfcompat.Provider{
			{Name: "simple", Factory: providers.SimpleProvider},
		},
		Config: sshConfig(c, map[string]string{"src": src}),
	})
}
