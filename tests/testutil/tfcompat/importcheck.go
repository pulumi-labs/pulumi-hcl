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
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/converter"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/pkg/v3/codegen/convert"
	"github.com/pulumi/pulumi/pkg/v3/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// runImportCheck re-runs the case through the TF-state import flow: the
// terraform state produced by the tofu-side apply is converted with the HCL
// converter's ConvertState, imported into a fresh Pulumi stack, and the next
// preview of the case's program must propose no changes — mirroring the
// promise that if `tofu plan` is clean after `tofu apply`, then `pulumi
// preview` is clean after `pulumi import --from hcl`.
//
// The converter runs in-process against a harness-owned mapper server serving
// the case's bridged provider mappings: the engine-side mapper the CLI flow
// would provide can only consult installed plugins, while the harness's
// providers are attached in-process. The end-to-end CLI flow (converter
// discovery, engine mapper) is covered separately by tests/importe2e against
// real released plugins.
func runImportCheck(
	t *testing.T, c Case, stage int, files map[string]string, store *tfexec.ImportStore, tfStateDir string,
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
				t.Skip("TODO[github.com/pulumi-labs/pulumi-hcl#167]: state-import check does not support plugin-framework providers yet")
			}
		}
		statePath := filepath.Join(tfStateDir, "terraform.tfstate")
		stateJSON, err := os.ReadFile(statePath)
		require.NoError(t, err)

		var state struct {
			Resources []struct {
				Mode   string `json:"mode"`
				Type   string `json:"type"`
				Module string `json:"module"`
			} `json:"resources"`
		}
		require.NoError(t, json.Unmarshal(stateJSON, &state))
		// Module-nested resources and terraform_data cannot import completely
		// yet (pulumi-labs/pulumi-hcl#167), so their previews would propose
		// creates.
		for _, r := range state.Resources {
			if r.Mode != "managed" {
				continue
			}
			if r.Module != "" {
				t.Skip("state has module-nested resources; import does not support modules yet")
			}
			if r.Type == "terraform_data" {
				t.Skip("state has terraform_data resources; import does not support the builtin yet")
			}
		}

		infos := make(map[string]tfbridge.ProviderInfo, len(c.Providers))
		for _, p := range c.Providers {
			infos[p.Name] = pulexec.BridgedProvider(t, p.Name, p.Factory(), p.Customize)
		}

		cancel := make(chan bool)
		handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
			Cancel: cancel,
			Init: func(srv *grpc.Server) error {
				convert.MapperRegistration(convert.NewMapperServer(providerInfoMapper{infos: infos}))(srv)
				return nil
			},
		})
		require.NoError(t, err)
		t.Cleanup(func() { close(cancel); <-handle.Done })

		// The driver's project directory supplies the sdks/ descriptors the
		// converter derives parameterization from, so it must exist first. Its
		// providers share the case's store, so import-time Reads reconstruct
		// the attributes the tofu side created.
		pulProvs := buildPulumiProviders(t, c.Providers, &tfexec.Recorder{}, store)
		d := pulexec.NewDriver(t, pulProvs, c.Config)
		d.WriteProgram(t, files)

		resp, err := converter.New().ConvertState(t.Context(), &plugin.ConvertStateRequest{
			MapperTarget: fmt.Sprintf("127.0.0.1:%d", handle.Port),
			Args:         []string{statePath, d.Dir()},
		})
		require.NoError(t, err)
		for _, d := range resp.Diagnostics {
			t.Logf("ConvertState diagnostic: %s: %s", d.Summary, d.Detail)
		}
		require.False(t, resp.Diagnostics.HasErrors(), "ConvertState reported error diagnostics")

		imp := importFile{Resources: make([]importSpec, 0, len(resp.Resources))}
		for _, r := range resp.Resources {
			spec := importSpec{
				Type:              r.Type,
				Name:              r.Name,
				ID:                r.ID,
				Version:           r.Version,
				PluginDownloadURL: r.PluginDownloadURL,
				LogicalName:       r.LogicalName,
				Component:         r.IsComponent,
				Remote:            r.IsRemote,
			}
			if p := r.Parameterization; p != nil {
				spec.Parameterization = &importParameterization{
					PluginName:    p.PluginName,
					PluginVersion: p.PluginVersion,
					Value:         p.Value,
				}
			}
			imp.Resources = append(imp.Resources, spec)
		}
		impJSON, err := json.MarshalIndent(imp, "", "  ")
		require.NoError(t, err)
		impPath := filepath.Join(t.TempDir(), "import.json")
		require.NoError(t, os.WriteFile(impPath, impJSON, 0o600))

		out, err := d.ImportFromFile(t, files, impPath)
		require.NoErrorf(t, err, "pulumi import failed:\n%s", out)

		steps, err := d.PreviewSteps(t)
		require.NoError(t, err)
		for _, step := range steps {
			if step.Op == "same" {
				continue
			}
			// The stack shell, providers, and module component shells are not
			// imported and carry no provider state, so their creates are
			// benign. Matched via the URN: plan steps do not always populate
			// the type field.
			leaf := resource.URN(step.URN).Type().String()
			if step.Op == "create" &&
				(leaf == "pulumi:pulumi:Stack" ||
					strings.HasPrefix(leaf, "pulumi:providers:") ||
					strings.HasPrefix(leaf, "components:")) {
				continue
			}
			t.Errorf("unexpected %q step for %s in the preview after import", step.Op, step.URN)
		}
	})
}

// providerInfoMapper is a convert.Mapper serving the bridged ProviderInfo
// mappings of the case's in-memory providers, exactly as an installed bridged
// plugin would over GetMapping.
type providerInfoMapper struct {
	infos map[string]tfbridge.ProviderInfo
}

func (m providerInfoMapper) GetMapping(
	_ context.Context, provider string, _ *convert.MapperPackageHint, _ string,
) ([]byte, error) {
	info, ok := m.infos[provider]
	if !ok {
		return nil, nil
	}
	return json.Marshal(tfbridge.MarshalProviderInfo(&info))
}

// importFile mirrors the JSON format consumed by `pulumi import --file`
// (pkg/cmd/pulumi/operations/import.go in pulumi/pulumi).
type importFile struct {
	Resources []importSpec `json:"resources,omitempty"`
}

type importSpec struct {
	Type              string                  `json:"type"`
	Name              string                  `json:"name"`
	ID                string                  `json:"id,omitempty"`
	Version           string                  `json:"version,omitempty"`
	PluginDownloadURL string                  `json:"pluginDownloadUrl,omitempty"`
	LogicalName       string                  `json:"logicalName,omitempty"`
	Component         bool                    `json:"component,omitempty"`
	Remote            bool                    `json:"remote,omitempty"`
	Parameterization  *importParameterization `json:"parameterization,omitempty"`
}

type importParameterization struct {
	PluginName    string `json:"pluginName"`
	PluginVersion string `json:"pluginVersion"`
	Value         []byte `json:"value"`
}
