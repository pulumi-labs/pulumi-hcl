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

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfcompat"
	"github.com/stretchr/testify/require"
)

// TestL1FileTilde pins that the file-reading functions expand a leading `~` to
// the user's home directory, matching OpenTofu's openFile. The fixture reads a
// file via `~/<name>`; OpenTofu reads it and reports it exists, so pulumi-hcl
// must too. The test writes the referenced file into the shared $HOME both
// runtimes see and removes it afterwards. The outputs are the file's literal
// contents and existence booleans, neither of which embeds a machine-specific
// path, so the comparison is stable across hosts.
func TestL1FileTilde(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	name := fmt.Sprintf(".pulumi_hcl_tfcompat_tilde_%d.txt", os.Getpid())
	abs := filepath.Join(home, name)
	require.NoError(t, os.WriteFile(abs, []byte("from-home"), 0o600))
	t.Cleanup(func() { _ = os.Remove(abs) })

	program := fmt.Sprintf(`
output "content" { value = file("~/%[1]s") }
output "b64"     { value = filebase64("~/%[1]s") }
output "exists"  { value = fileexists("~/%[1]s") }
`, name)

	tfcompat.RunCase(t, "l1_file_tilde", tfcompat.Case{
		Stages: []tfcompat.Stage{
			{Files: map[string]string{"main.tf": program}},
		},
	})
}
