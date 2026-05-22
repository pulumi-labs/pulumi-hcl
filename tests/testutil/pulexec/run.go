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

package pulexec

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sync"
	"testing"

	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/optnewstack"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

var (
	buildHCLOnce sync.Once
	hclBinDir    string
	hclBuildErr  error
)

// ensureHCLLanguagePlugin builds pulumi-language-hcl once per test process and
// returns the directory containing the binary so it can be prepended to PATH.
//
// We can't use t.TempDir here: the binary lives for the entire test process
// across many tests, but t.TempDir is cleaned up when each test ends.
func ensureHCLLanguagePlugin(t *testing.T) string {
	t.Helper()
	buildHCLOnce.Do(func() {
		dir, err := os.MkdirTemp("", "pulumi-language-hcl-*") //nolint:usetesting // binary outlives any single test
		if err != nil {
			hclBuildErr = fmt.Errorf("creating temp dir: %w", err)
			return
		}
		bin := filepath.Join(dir, "pulumi-language-hcl")
		cmd := exec.Command("go", "build", "-o", bin, "github.com/pulumi-labs/pulumi-hcl/cmd/pulumi-language-hcl")
		out, err := cmd.CombinedOutput()
		if err != nil {
			hclBuildErr = fmt.Errorf("building pulumi-language-hcl: %w\n%s", err, out)
			return
		}
		hclBinDir = dir
	})
	require.NoError(t, hclBuildErr)
	return hclBinDir
}

// Provider pairs a provider name with its bridged info.
type Provider struct {
	Name string
	Info tfbridge.ProviderInfo
}

// Result holds the outputs and resource state from a Pulumi deployment.
type Result struct {
	Outputs   map[string]string
	Resources []apitype.ResourceV3
}

// Driver wraps a long-lived pulumitest project so callers can run multiple
// `pulumi up` cycles against the same stack — required for tests that verify
// behavior across changes (e.g. lifecycle.replace_triggered_by).
type Driver struct {
	pt  *pulumitest.PulumiTest
	dir string
}

// NewDriver builds the project dir, attaches the bridged providers, and sets
// any stack config. Call Driver.Apply once per stage.
func NewDriver(t *testing.T, provs []Provider, config map[string]string) *Driver {
	t.Helper()

	binDir := ensureHCLLanguagePlugin(t)
	dir := t.TempDir()

	// The project name is used as the default namespace for user config. It
	// must not collide with any attached provider name, or user config like
	// "<project>:foo" would be misrouted to the provider.
	pulumiYAML := `name: tfcompat
runtime: hcl
backend:
  url: file://` + filepath.Join(dir, "state") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Pulumi.yaml"), []byte(pulumiYAML), 0o600))

	opts := append(make([]opttest.Option, 0, 5+len(provs)),
		opttest.Env("PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION", "true"),
		opttest.Env("PATH", binDir+string(os.PathListSeparator)+os.Getenv("PATH")),
		opttest.TestInPlace(),
		opttest.SkipInstall(),
		// Cleanup destroy fails on prevent_destroy cases; temp dir is
		// discarded by t.TempDir anyway.
		opttest.NewStackOptions(optnewstack.DisableAutoDestroy()),
	)
	for _, p := range provs {
		info := p.Info
		opts = append(opts, opttest.AttachProvider(
			p.Name,
			func(ctx context.Context, pt providers.PulumiTest) (providers.Port, error) {
				handle, err := startProvider(ctx, info)
				if err != nil {
					return 0, err
				}
				return providers.Port(handle.Port), nil
			},
		))
	}

	pt := pulumitest.NewPulumiTest(t, dir, opts...)
	for k, v := range config {
		pt.SetConfig(t, k, v)
	}
	return &Driver{pt: pt, dir: dir}
}

// Apply writes programFiles into the project dir (replacing any prior .tf
// program files) and runs `pulumi up`. Returns stack outputs and resource
// state from the resulting deployment.
func (d *Driver) Apply(t *testing.T, programFiles map[string]string) Result {
	t.Helper()

	require.NoError(t, removeProgramFiles(d.dir))
	d.writeFiles(t, programFiles)

	upResult := d.pt.Up(t)

	outputs := make(map[string]string, len(upResult.Outputs))
	for k, v := range upResult.Outputs {
		if s, ok := v.Value.(string); ok {
			outputs[k] = s
		} else {
			raw, err := json.Marshal(v.Value)
			require.NoError(t, err)
			outputs[k] = string(raw)
		}
	}

	exported := d.pt.ExportStack(t)
	var deployment apitype.DeploymentV3
	require.NoError(t, json.Unmarshal(exported.Deployment, &deployment))

	return Result{
		Outputs:   outputs,
		Resources: deployment.Resources,
	}
}

// TryApply runs `pulumi up` and returns the error instead of fataling. State
// is exported regardless of error so callers can inspect post-failure state.
func (d *Driver) TryApply(t *testing.T, programFiles map[string]string) (Result, error) {
	t.Helper()

	require.NoError(t, removeProgramFiles(d.dir))
	d.writeFiles(t, programFiles)

	var upResult auto.UpResult
	cap := newCaptureT(t)
	done := make(chan struct{})
	go func() {
		// pt.Up's fatal calls runtime.Goexit; isolate it in this goroutine.
		defer close(done)
		upResult = d.pt.Up(cap)
	}()
	<-done
	var upErr error
	if cap.Failed() {
		upErr = fmt.Errorf("pulumi up: %s", cap.Logs())
	}

	outputs := make(map[string]string, len(upResult.Outputs))
	for k, v := range upResult.Outputs {
		if s, ok := v.Value.(string); ok {
			outputs[k] = s
		} else {
			raw, err := json.Marshal(v.Value)
			require.NoError(t, err)
			outputs[k] = string(raw)
		}
	}

	exported := d.pt.ExportStack(t)
	var deployment apitype.DeploymentV3
	require.NoError(t, json.Unmarshal(exported.Deployment, &deployment))

	return Result{
		Outputs:   outputs,
		Resources: deployment.Resources,
	}, upErr
}

// Preview runs `pulumi preview` and returns the error (nil on success).
// Same captureT indirection as TryApply.
func (d *Driver) Preview(t *testing.T, programFiles map[string]string) error {
	t.Helper()

	require.NoError(t, removeProgramFiles(d.dir))
	d.writeFiles(t, programFiles)

	cap := newCaptureT(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.pt.Preview(cap)
	}()
	<-done
	if cap.Failed() {
		return fmt.Errorf("pulumi preview: %s", cap.Logs())
	}
	return nil
}

func (d *Driver) writeFiles(t *testing.T, programFiles map[string]string) {
	t.Helper()
	for path, content := range programFiles {
		fullPath := filepath.Join(d.dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
	}
}

// removeProgramFiles deletes every regular file under dir except Pulumi.yaml
// and entries inside the `state/` backend directory.
func removeProgramFiles(dir string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	for _, e := range entries {
		name := e.Name()
		if name == "Pulumi.yaml" || name == "state" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(dir, name)); err != nil {
			return err
		}
	}
	return nil
}

func startProvider(ctx context.Context, providerInfo tfbridge.ProviderInfo) (*rpcutil.ServeHandle, error) {
	prov, err := providerServerFromInfo(ctx, providerInfo)
	if err != nil {
		return nil, fmt.Errorf("providerServerFromInfo failed: %w", err)
	}

	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Init: func(srv *grpc.Server) error {
			pulumirpc.RegisterResourceProviderServer(srv, prov)
			return nil
		},
	})
	if err != nil {
		return nil, fmt.Errorf("rpcutil.ServeWithOptions failed: %w", err)
	}

	return &handle, nil
}
