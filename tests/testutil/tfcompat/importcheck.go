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

package tfcompat

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi-hcl/pkg/util/encryption"
	"github.com/pulumi/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfexec"
	"github.com/pulumi/pulumi-hcl/vendored/addrs"
	"github.com/pulumi/pulumi-hcl/vendored/statefile"
	"github.com/stretchr/testify/require"
)

// runImportCheck re-runs the case through the TF-state import flow: the
// terraform state produced by the tofu-side apply is fed to
// `pulumi import --from hcl` against a fresh Pulumi stack, and the next
// preview and up of the case's program must touch no resource — mirroring the
// promise that if `tofu plan` is clean after `tofu apply`, then Pulumi
// operations are no-ops after `pulumi import --from hcl`.
//
// The CLI drives the whole flow: it spawns the converter (an in-process server
// behind a PATH shim) and resolves mappings through its own engine-side
// mapper, which reaches the case's providers via PULUMI_DEBUG_PROVIDERS. The
// same flow against real released plugins is covered by tests/importe2e.
func runImportCheck(
	t *testing.T, c Case, stage int, files map[string]string, tfStateDir string,
) {
	t.Helper()

	// Later stages get an explicit suffix: Go's own duplicate-name suffix
	// ("#01") breaks the file-backend URL, which parses "#" as a fragment.
	name := "state-import"
	if stage > 0 {
		name = fmt.Sprintf("state-import-%d", stage)
	}
	t.Run(name, func(t *testing.T) {
		if c.SkipImport != "" {
			t.Skipf("state check skipped: %s", c.SkipImport)
		}
		for _, p := range c.Providers {
			if p.PFFactory != nil {
				t.Skip("TODO[github.com/pulumi/pulumi-hcl#167]: state-import check does not support plugin-framework providers yet")
			}
		}
		statePath := filepath.Join(tfStateDir, "terraform.tfstate")
		stateJSON, err := os.ReadFile(statePath)
		require.NoError(t, err)

		f, err := statefile.Read(bytes.NewReader(stateJSON), encryption.StateEncryptionDisabled())
		require.NoError(t, err)
		// Module-nested resources and terraform_data cannot import completely
		// yet (pulumi/pulumi-hcl#167), so their previews would propose
		// creates.
		for _, mod := range f.State.Modules {
			for _, res := range mod.Resources {
				if res.Addr.Resource.Mode != addrs.ManagedResourceMode {
					continue
				}
				if !mod.Addr.IsRoot() {
					t.Skip("state has module-nested resources; import does not support modules yet")
				}
				if res.Addr.Resource.Type == "terraform_data" {
					t.Skip("state has terraform_data resources; import does not support the builtin yet")
				}
			}
		}

		// Import into a fresh stack.
		pulProvs := buildPulumiProviders(t, c.Providers, &tfexec.Recorder{})
		d := pulexec.NewDriver(t, pulProvs, c.Config)
		out, err := d.Import(t, files, statePath)
		require.NoErrorf(t, err, "pulumi import --from hcl failed:\n%s", out)

		// A clean import means the next operations plan and perform no
		// resource changes, observed at the provider RPC boundary: a preview
		// surfaces planned creates and updates as Create/Update with
		// preview=true, and planned deletes — invisible to a preview at that
		// boundary — would execute on the real up. The stack shell, default
		// providers, and module component shells are still created, but none
		// of those reach a resource provider's mutating RPCs.
		require.NoError(t, d.Preview(t, files))
		require.Empty(t, d.Mutations(), "preview after import proposed resource changes")

		res, err := d.TryApply(t, files)
		require.NoErrorf(t, err, "up after import failed:\n%s", res.Output)
		require.Empty(t, d.Mutations(), "up after import performed resource changes")
	})
}
