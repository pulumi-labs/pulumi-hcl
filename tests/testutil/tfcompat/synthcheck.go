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
	"fmt"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/converter"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/stretchr/testify/require"
)

// runSynthCheck re-runs the case through the direct state-synthesis flow: the
// terraform state produced by the tofu-side apply is translated into a Pulumi
// deployment with SynthesizeStateDeployment, loaded with `pulumi stack
// import`, and the next preview of the case's program must propose no
// changes. Unlike the state-import check no provider Read ever runs — the
// synthesized outputs are compared against the program directly — so this
// exercises the attribute translation rather than the providers' importers.
func runSynthCheck(
	t *testing.T, c Case, stage int, files map[string]string, store *tfexec.ImportStore, tfStateDir string,
) {
	t.Helper()

	// Later stages get an explicit suffix: Go's own duplicate-name suffix
	// ("#01") breaks the file-backend URL, which parses "#" as a fragment.
	name := "state-synthesis"
	if stage > 0 {
		name = fmt.Sprintf("state-synthesis-%d", stage)
	}
	t.Run(name, func(t *testing.T) {
		statePath := importableTFState(t, c, tfStateDir)

		infos := make(map[string]tfbridge.ProviderInfo, len(c.Providers))
		for _, p := range c.Providers {
			infos[p.Name] = pulexec.BridgedProvider(t, p.Name, p.Factory(), p.Customize)
		}

		// The driver's project directory supplies the sdks/ descriptors that
		// synthesis derives parameterized provider entries from.
		pulProvs := buildPulumiProviders(t, c.Providers, &tfexec.Recorder{}, store)
		d := pulexec.NewDriver(t, pulProvs, c.Config)
		d.WriteProgram(t, files)

		dep, diags, err := converter.SynthesizeStateDeployment(
			t.Context(), infos, statePath, d.Dir(), d.ProjectName(), d.StackName(t))
		require.NoError(t, err)
		for _, dg := range diags {
			t.Logf("synthesis diagnostic: %s: %s", dg.Summary, dg.Detail)
		}
		require.False(t, diags.HasErrors(), "synthesis reported error diagnostics")

		require.NoError(t, d.StackImport(t, *dep), "stack import failed")

		steps, err := d.PreviewSteps(t)
		require.NoError(t, err)
		assertCleanPostImportPreview(t, steps)
	})
}
