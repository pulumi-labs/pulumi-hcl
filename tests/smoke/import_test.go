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

package smoke_test

import (
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

// random_uuid needs no credentials and, unlike most of the random provider,
// implements import — by its UUID, which is exactly the state's `id` attribute.
const importProgram = `terraform {
  required_providers {
    random = {
      source  = "hashicorp/random"
      version = "3.6.0"
    }
  }
}

resource "random_uuid" "example" {
}
`

// TestImportFromHCL drives the real `tofu` and `pulumi` binaries through
// `pulumi import --from hcl`, encoding its core promise: if `tofu plan` shows
// no diff, the first `pulumi preview` after import must not either.
func TestImportFromHCL(t *testing.T) {
	t.Parallel()
	tofuBin := lookPath(t, "tofu", "terraform")
	pulumiBin := lookPath(t, "pulumi")
	lookPath(t, "pulumi-language-hcl")
	lookPath(t, "pulumi-converter-hcl")

	// The file backend requires its directory to exist before `stack init`.
	base := t.TempDir()
	require.NoError(t, os.MkdirAll(filepath.Join(base, "state"), 0o755))
	env := append(os.Environ(),
		"PULUMI_CONFIG_PASSPHRASE=test",
		"PULUMI_BACKEND_URL=file://"+filepath.Join(base, "state"),
		// Never reach out for the language plugin: it must already be on PATH.
		"PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION=true",
	)

	tfDir := filepath.Join(base, "tf")
	require.NoError(t, os.MkdirAll(tfDir, 0o755))
	writeFile(t, filepath.Join(tfDir, "main.tf"), importProgram)
	run(t, tfDir, env, tofuBin, "init", "-input=false")
	run(t, tfDir, env, tofuBin, "apply", "-auto-approve", "-input=false")

	statePath := filepath.Join(tfDir, "terraform.tfstate")
	require.FileExists(t, statePath, "tofu apply should have produced state")

	// The premise: `tofu plan` is clean, so `pulumi preview` must be too.
	run(t, tfDir, env, tofuBin, "plan", "-detailed-exitcode", "-input=false")

	pulDir := filepath.Join(base, "pulumi")
	require.NoError(t, os.MkdirAll(pulDir, 0o755))
	writeFile(t, filepath.Join(pulDir, "Pulumi.yaml"), "name: import-e2e\nruntime: hcl\n")
	writeFile(t, filepath.Join(pulDir, "main.tf"), importProgram)

	// `pulumi install` writes sdks/random/hcl.sdk.json, the descriptor
	// ConvertState reads to build the parameterized import.
	run(t, pulDir, env, pulumiBin, "install")
	require.FileExists(t, filepath.Join(pulDir, "sdks", "random", "hcl.sdk.json"),
		"pulumi install should have written the random SDK descriptor")

	run(t, pulDir, env, pulumiBin, "stack", "init", "dev")

	// --protect=false: HCL programs cannot express the protect option, so the
	// default (protected) would make every first preview diff on it.
	run(t, pulDir, env, pulumiBin, "import", "--from", "hcl", statePath, "--protect=false", "--yes")

	// No create/delete/replace allowed: the import must land resources under
	// the same parameterized provider the runtime uses. In-place updates are
	// tolerated: the provider's live importer can materialize unset attributes
	// differently from the TF state (e.g. random's `keepers: null` imports as
	// `{}`; cf. pulumi-converter-terraform#69/#72).
	steps := previewSteps(t, pulDir, env, pulumiBin)
	require.NotEmpty(t, steps)
	for _, step := range steps {
		require.Containsf(t, []string{"same", "update"}, step.Op,
			"unexpected %q step for %s: the imported resource must not be created/deleted/replaced", step.Op, step.URN)
	}

	// TODO(pulumi-labs/pulumi-hcl#167): once import round-trips unset
	// attributes faithfully, drop the `up` and assert --expect-no-changes
	// directly after import.
	run(t, pulDir, env, pulumiBin, "up", "--yes", "--skip-preview")
	run(t, pulDir, env, pulumiBin, "preview", "--expect-no-changes")
}

type previewStep struct {
	Op  string `json:"op"`
	URN string `json:"urn"`
}

func previewSteps(t *testing.T, dir string, env []string, pulumiBin string) []previewStep {
	t.Helper()
	out := runOut(t, dir, env, pulumiBin, "preview", "--json")
	var plan struct {
		Steps []previewStep `json:"steps"`
	}
	require.NoError(t, json.Unmarshal([]byte(out), &plan), "parsing preview --json output")
	return plan.Steps
}

func lookPath(t *testing.T, names ...string) string {
	t.Helper()
	for _, name := range names {
		if p, err := exec.LookPath(name); err == nil {
			return p
		}
	}
	t.Skipf("none of %v found on PATH", names)
	return ""
}

func writeFile(t *testing.T, path, content string) {
	t.Helper()
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))
}

func run(t *testing.T, dir string, env []string, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	out, err := cmd.CombinedOutput()
	t.Logf("$ %s %s\n%s", filepath.Base(name), strings.Join(args, " "), out)
	require.NoErrorf(t, err, "%s %s failed", filepath.Base(name), strings.Join(args, " "))
}

// runOut returns stdout separately so callers can parse it as JSON.
func runOut(t *testing.T, dir string, env []string, name string, args ...string) string {
	t.Helper()
	cmd := exec.Command(name, args...)
	cmd.Dir = dir
	cmd.Env = env
	var stderr strings.Builder
	cmd.Stderr = &stderr
	out, err := cmd.Output()
	t.Logf("$ %s %s\n%s%s", filepath.Base(name), strings.Join(args, " "), out, stderr.String())
	require.NoErrorf(t, err, "%s %s failed", filepath.Base(name), strings.Join(args, " "))
	return string(out)
}
