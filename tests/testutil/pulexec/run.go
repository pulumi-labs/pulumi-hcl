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
	"path/filepath"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/server"
	"github.com/pulumi/providertest/providers"
	"github.com/pulumi/providertest/pulumitest"
	"github.com/pulumi/providertest/pulumitest/optnewstack"
	"github.com/pulumi/providertest/pulumitest/opttest"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/auto"
	"github.com/pulumi/pulumi/sdk/v3/go/auto/optdestroy"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
)

// serveLanguageHost serves the pulumi-language-hcl runtime in-process on an
// ephemeral port and returns that port. The engine attaches to it via
// PULUMI_DEBUG_LANGUAGES instead of spawning the plugin binary, so the runtime
// under test runs in the test process (no build step, debuggable, covered).
//
// The engine address is unknown at construction; it arrives per `pulumi`
// invocation through the host's Handshake RPC. Each Driver gets its own host
// so concurrent tests never share an engine connection.
func serveLanguageHost(t *testing.T) int {
	t.Helper()
	cancel := make(chan bool)
	t.Cleanup(func() { close(cancel) })
	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Cancel: cancel,
		Init: func(srv *grpc.Server) error {
			host, err := server.NewLanguageHost("")
			if err != nil {
				return err
			}
			t.Cleanup(func() { contract.IgnoreClose(host) })
			pulumirpc.RegisterLanguageRuntimeServer(srv, host)
			return nil
		},
	})
	require.NoError(t, err)
	return handle.Port
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
	pt        *pulumitest.PulumiTest
	dir       string
	providers []string
	// lastProgramFiles are the program-file paths written by the previous
	// writeFiles call, removed before the next stage so a stage that drops a
	// file doesn't inherit a stale copy.
	lastProgramFiles []string
}

// NewDriver builds the project dir, attaches the bridged providers, and sets
// any stack config. Call Driver.Apply once per stage.
func NewDriver(t *testing.T, provs []Provider, config map[string]string) *Driver {
	t.Helper()

	hostPort := serveLanguageHost(t)
	dir := t.TempDir()

	provNames := make([]string, len(provs))
	for i, p := range provs {
		provNames[i] = p.Name
	}

	// The project name is used as the default namespace for user config. It
	// must not collide with any attached provider name, or user config like
	// "<project>:foo" would be misrouted to the provider.
	pulumiYAML := `name: tfcompat
runtime: hcl
backend:
  url: file://` + filepath.Join(dir, "state") + "\n"
	require.NoError(t, os.WriteFile(filepath.Join(dir, "Pulumi.yaml"), []byte(pulumiYAML), 0o600))

	opts := append(
		make([]opttest.Option, 0, 5+len(provs)),
		opttest.Env("PULUMI_DISABLE_AUTOMATIC_PLUGIN_ACQUISITION", "true"),
		opttest.Env("PULUMI_DEBUG_LANGUAGES", fmt.Sprintf("hcl:%d", hostPort)),
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
	return &Driver{pt: pt, dir: dir, providers: provNames}
}

// Apply writes programFiles into the project dir (replacing any prior .tf
// program files) and runs `pulumi up`. Returns stack outputs and resource
// state from the resulting deployment.
// Dir returns the program directory where pulumi runs and program files are
// written. Tests use this to scrub the temp path out of values that bake it
// in (e.g. path.cwd) before cross-driver comparison.
func (d *Driver) Dir() string { return d.dir }

func (d *Driver) Apply(t *testing.T, programFiles map[string]string) Result {
	t.Helper()

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

// --run-program re-runs the language host during destroy so BeforeDelete
// hooks (destroy-time provisioners) can fire.
func (d *Driver) Destroy(t *testing.T, programFiles map[string]string) error {
	t.Helper()

	d.writeFiles(t, programFiles)

	cap := newCaptureT(t)
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.pt.Destroy(cap, optdestroy.RunProgram(true))
	}()
	<-done
	if cap.Failed() {
		return fmt.Errorf("pulumi destroy: %s", cap.Logs())
	}
	return nil
}

func (d *Driver) writeFiles(t *testing.T, programFiles map[string]string) {
	t.Helper()
	// Remove the previous stage's program files so a stage that drops a file doesn't
	// inherit a stale copy.
	for _, path := range d.lastProgramFiles {
		require.NoError(t, os.RemoveAll(filepath.Join(d.dir, path)))
	}
	d.lastProgramFiles = d.lastProgramFiles[:0]
	for path, content := range programFiles {
		fullPath := filepath.Join(d.dir, path)
		require.NoError(t, os.MkdirAll(filepath.Dir(fullPath), 0o755))
		require.NoError(t, os.WriteFile(fullPath, []byte(content), 0o600))
		d.lastProgramFiles = append(d.lastProgramFiles, path)
	}
	d.writeStubSDKs(t)
}

// writeStubSDKs materializes a minimal sdks/<provider>/hcl.sdk.json for each
// attached provider. In production this file is written by `pulumi install`
// (via GeneratePackage), but the harness uses AttachProvider, so the
// terraform-provider plugin that `pulumi install` would normally invoke is
// never run. The descriptor is intentionally unparameterized — AttachProvider
// short-circuits plugin lookup to the in-process gRPC server.
func (d *Driver) writeStubSDKs(t *testing.T) {
	t.Helper()
	for _, name := range d.providers {
		sdkDir := filepath.Join(d.dir, "sdks", name)
		require.NoError(t, os.MkdirAll(sdkDir, 0o755))
		desc := fmt.Sprintf(`{"name":%q,"kind":"resource"}`+"\n", name)
		require.NoError(t, os.WriteFile(
			filepath.Join(sdkDir, "hcl.sdk.json"), []byte(desc), 0o600,
		))
	}
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
