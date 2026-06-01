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

// Package tfcompat is the Terraform-compatibility test harness. RunCase loads
// a `.tf` program from testdata/cases/<name>/ and runs it through two paths,
// asserting they produce identical outputs and provoke identical provider
// operations:
//
//	Path A: real `tofu apply` against the in-memory TF providers (via reattach).
//	Path B: real `pulumi up` against the same providers (bridged), running the
//	        pulumi-language-hcl runtime.
//
// Recordings are captured by wrapping each *schema.Provider with tfexec.Wrap
// before either path sees it, so the comparisons are apples-to-apples.
//
// A case directory may either be flat (single apply) or contain numbered stage
// subdirs (0/, 1/, ...) for tests that need to drive multiple applies against
// the same stack — required when verifying behavior on subsequent changes
// (e.g. lifecycle.replace_triggered_by).
package tfcompat

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/require"
)

// Provider describes an in-memory TF provider that's available to both paths.
type Provider struct {
	Name    string
	Factory func() *schema.Provider
	// Customize, if non-nil, runs against the bridged ProviderInfo on the
	// Pulumi path so tests can apply non-default Pulumi-side renames (or any
	// other ProviderInfo tweak) to exercise the bridge mapping behaviour.
	// The TF path is unaffected.
	Customize func(*testing.T, *tfbridge.ProviderInfo)
}

// Case is the test description passed to RunCase.
type Case struct {
	Providers []Provider
	Config    map[string]string

	// AssertState, if set, runs after `pulumi up`. Use for assertions on
	// resource fields that aren't reachable via stack outputs (e.g. Protect).
	AssertState func(t *testing.T, resources []apitype.ResourceV3)

	// Stages, if non-nil, overrides disk-based stage loading. Use this when
	// you need per-stage expected errors or have inline program content.
	Stages []Stage
}

type StageMode int

const (
	StageApply   StageMode = iota // `tofu apply` / `pulumi up`; default
	StagePreview                  // `tofu plan` / `pulumi preview`
	StageDestroy                  // `tofu destroy` / `pulumi destroy`
)

// Stage is one operation within a Case.
type Stage struct {
	Files map[string]string
	Mode  StageMode
	// ExpectErr, if non-empty, requires both runtimes to fail with an error
	// containing this substring.
	ExpectErr string
}

// RunCase resolves testdata/cases/<caseName>/ relative to the calling test
// file, loads every regular file in that directory, and runs the comparison.
// If c.Stages is set the disk path is bypassed in favor of inline stages.
func RunCase(t *testing.T, caseName string, c Case) {
	t.Helper()

	if len(c.Stages) > 0 {
		runCaseStages(t, c)
		return
	}

	_, callerFile, _, _ := runtime.Caller(1)
	caseDir := filepath.Join(filepath.Dir(callerFile), "testdata", "cases", caseName)
	runCaseFromDir(t, caseDir, c)
}

// runCaseStages runs inline Case.Stages. Ops/outputs/AssertState are compared
// against the last successful apply stage; preview stages produce no state.
func runCaseStages(t *testing.T, c Case) {
	t.Helper()

	recA, recB := &tfexec.Recorder{}, &tfexec.Recorder{}
	tfProvs := make([]tfexec.Provider, len(c.Providers))
	pulProvs := make([]pulexec.Provider, len(c.Providers))
	for i, p := range c.Providers {
		tfProvs[i] = tfexec.Provider{Name: p.Name, Provider: tfexec.Wrap(p.Factory(), recA)}
		pulProvs[i] = pulexec.Provider{
			Name: p.Name,
			Info: pulexec.BridgedProvider(t, p.Name, tfexec.Wrap(p.Factory(), recB), p.Customize),
		}
	}

	tfDriver := tfexec.NewDriver(t, tfProvs)
	pulDriver := pulexec.NewDriver(t, pulProvs, c.Config)

	var lastOK pulexec.Result
	var lastOKTfOutputs map[string]string
	for i, stage := range c.Stages {
		var wg sync.WaitGroup
		wg.Add(2)

		var tfOut map[string]string
		var tfErr error
		var pulRes pulexec.Result
		var pulErr error
		go func() {
			defer wg.Done()
			switch stage.Mode {
			case StagePreview:
				tfErr = tfDriver.Plan(t, stage.Files, c.Config)
			case StageDestroy:
				tfErr = tfDriver.Destroy(t, stage.Files, c.Config)
			default:
				tfOut, tfErr = tfDriver.TryApply(t, stage.Files, c.Config)
			}
		}()
		go func() {
			defer wg.Done()
			switch stage.Mode {
			case StagePreview:
				pulErr = pulDriver.Preview(t, stage.Files)
			case StageDestroy:
				pulErr = pulDriver.Destroy(t, stage.Files)
			default:
				pulRes, pulErr = pulDriver.TryApply(t, stage.Files)
			}
		}()
		wg.Wait()

		tfLabel, pulLabel := "tofu apply", "pulumi up"
		switch stage.Mode {
		case StagePreview:
			tfLabel, pulLabel = "tofu plan", "pulumi preview"
		case StageDestroy:
			tfLabel, pulLabel = "tofu destroy", "pulumi destroy"
		}

		if stage.ExpectErr != "" {
			require.Errorf(t, tfErr,
				"stage %d: %s was expected to fail with %q", i, tfLabel, stage.ExpectErr)
			require.Errorf(t, pulErr,
				"stage %d: %s was expected to fail with %q", i, pulLabel, stage.ExpectErr)
			require.Containsf(t, tfErr.Error(), stage.ExpectErr,
				"stage %d: %s error did not contain expected substring", i, tfLabel)
			require.Containsf(t, pulErr.Error(), stage.ExpectErr,
				"stage %d: %s error did not contain expected substring", i, pulLabel)
			continue
		}

		require.NoErrorf(t, tfErr, "stage %d: %s failed unexpectedly", i, tfLabel)
		require.NoErrorf(t, pulErr, "stage %d: %s failed unexpectedly", i, pulLabel)

		if stage.Mode == StageApply {
			lastOK = pulRes
			lastOKTfOutputs = tfOut
		}
	}

	if lastOKTfOutputs != nil {
		require.Equal(t,
			scrubTmpDir(lastOKTfOutputs, tfDriver.Dir()),
			scrubTmpDir(lastOK.Outputs, pulDriver.Dir()),
			"stack outputs differ between tofu apply and pulumi up")
		require.Equal(t, recA.Ops(), recB.Ops(),
			"provider operations differ between tofu apply and pulumi up")
		if c.AssertState != nil {
			c.AssertState(t, lastOK.Resources)
		}
	}
}

// runCaseFromDir runs a case whose program files live in caseDir. Exposed for
// self-tests that drive the harness with a t.TempDir() rather than an
// on-disk testdata directory.
func runCaseFromDir(t *testing.T, caseDir string, c Case) {
	t.Helper()

	stages := loadStages(t, caseDir)

	recA, recB := &tfexec.Recorder{}, &tfexec.Recorder{}

	tfProvs := make([]tfexec.Provider, len(c.Providers))
	pulProvs := make([]pulexec.Provider, len(c.Providers))
	for i, p := range c.Providers {
		tfProvs[i] = tfexec.Provider{Name: p.Name, Provider: tfexec.Wrap(p.Factory(), recA)}
		pulProvs[i] = pulexec.Provider{
			Name: p.Name,
			Info: pulexec.BridgedProvider(t, p.Name, tfexec.Wrap(p.Factory(), recB), p.Customize),
		}
	}

	tfDriver := tfexec.NewDriver(t, tfProvs)
	pulDriver := pulexec.NewDriver(t, pulProvs, c.Config)

	var outA map[string]string
	var pulResult pulexec.Result
	for i, files := range stages {
		var wg sync.WaitGroup
		wg.Add(2)
		go func() {
			defer wg.Done()
			outA = tfDriver.Apply(t, files, c.Config)
		}()
		go func() {
			defer wg.Done()
			pulResult = pulDriver.Apply(t, files)
		}()
		wg.Wait()
		if t.Failed() {
			t.Logf("stage %d failed", i)
			return
		}
	}

	require.Equal(t,
		scrubTmpDir(outA, tfDriver.Dir()),
		scrubTmpDir(pulResult.Outputs, pulDriver.Dir()),
		"stack outputs differ between tofu apply and pulumi up")
	require.Equal(t, recA.Ops(), recB.Ops(), "provider operations differ between tofu apply and pulumi up")

	if c.AssertState != nil {
		c.AssertState(t, pulResult.Resources)
	}
}

// scrubTmpDir replaces driver-specific temp paths in output values with the
// sentinel "<TMPDIR>" so cross-driver comparison ignores the fact that tofu
// and pulumi run from different temp directories. Symlink-resolved forms (on
// macOS, /var/folders/... is a symlink to /private/var/folders/...) are also
// scrubbed.
func scrubTmpDir(outputs map[string]string, dir string) map[string]string {
	if dir == "" {
		return outputs
	}
	forms := []string{dir}
	if resolved, err := filepath.EvalSymlinks(dir); err == nil && resolved != dir {
		forms = append(forms, resolved)
	}
	out := make(map[string]string, len(outputs))
	for k, v := range outputs {
		for _, form := range forms {
			v = strings.ReplaceAll(v, form, "<TMPDIR>")
		}
		out[k] = v
	}
	return out
}

// loadStages returns one entry per apply: a case directory containing only
// numbered subdirs (0/, 1/, ...) becomes that many stages, in order; any other
// shape is a single stage built from the whole directory.
func loadStages(t *testing.T, caseDir string) []map[string]string {
	t.Helper()
	stages, err := loadStagesFS(caseDir)
	require.NoError(t, err)
	return stages
}

func loadStagesFS(caseDir string) ([]map[string]string, error) {
	info, err := os.Stat(caseDir)
	if err != nil {
		return nil, fmt.Errorf("case directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("case path %q is not a directory", caseDir)
	}

	entries, err := os.ReadDir(caseDir)
	if err != nil {
		return nil, err
	}

	stageDirs := make(map[int]string)
	for _, e := range entries {
		if !e.IsDir() {
			stageDirs = nil
			break
		}
		n, err := strconv.Atoi(e.Name())
		if err != nil || n < 0 {
			stageDirs = nil
			break
		}
		stageDirs[n] = filepath.Join(caseDir, e.Name())
	}

	if len(stageDirs) == 0 {
		files, err := loadCaseFS(caseDir)
		if err != nil {
			return nil, err
		}
		return []map[string]string{files}, nil
	}

	keys := make([]int, 0, len(stageDirs))
	for k := range stageDirs {
		keys = append(keys, k)
	}
	sort.Ints(keys)
	stages := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		files, err := loadCaseFS(stageDirs[k])
		if err != nil {
			return nil, fmt.Errorf("stage %d: %w", k, err)
		}
		stages = append(stages, files)
	}
	return stages, nil
}

// loadCaseFS reads every regular file under caseDir and returns a map of
// relative-path → file contents. It errors if caseDir is missing, is not a
// directory, or contains no files.
func loadCaseFS(caseDir string) (map[string]string, error) {
	info, err := os.Stat(caseDir)
	if err != nil {
		return nil, fmt.Errorf("case directory: %w", err)
	}
	if !info.IsDir() {
		return nil, fmt.Errorf("case path %q is not a directory", caseDir)
	}

	files := make(map[string]string)
	err = filepath.WalkDir(caseDir, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			return nil
		}
		rel, relErr := filepath.Rel(caseDir, path)
		if relErr != nil {
			return relErr
		}
		content, readErr := os.ReadFile(path) //nolint:gosec // caseDir is test-controlled
		if readErr != nil {
			return readErr
		}
		files[rel] = string(content)
		return nil
	})
	if err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files found in case directory %q", caseDir)
	}
	return files, nil
}
