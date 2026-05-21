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
package tfcompat

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"testing"

	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/stretchr/testify/require"
)

// Provider describes an in-memory TF provider that's available to both paths.
type Provider struct {
	Name    string
	Factory func() *schema.Provider
}

// Case is the test description passed to RunCase.
type Case struct {
	Providers []Provider
	Config    map[string]string
}

// RunCase resolves testdata/cases/<caseName>/ relative to the calling test
// file, loads every regular file in that directory, and runs the comparison.
func RunCase(t *testing.T, caseName string, c Case) {
	t.Helper()

	_, callerFile, _, _ := runtime.Caller(1)
	caseDir := filepath.Join(filepath.Dir(callerFile), "testdata", "cases", caseName)
	runCaseFromDir(t, caseDir, c)
}

// runCaseFromDir runs a case whose program files live in caseDir. Exposed for
// self-tests that drive the harness with a t.TempDir() rather than an
// on-disk testdata directory.
func runCaseFromDir(t *testing.T, caseDir string, c Case) {
	t.Helper()

	files := loadCase(t, caseDir)

	recA, recB := &tfexec.Recorder{}, &tfexec.Recorder{}

	tfProvs := make([]tfexec.Provider, len(c.Providers))
	pulProvs := make([]pulexec.Provider, len(c.Providers))
	for i, p := range c.Providers {
		tfProvs[i] = tfexec.Provider{Name: p.Name, Provider: tfexec.Wrap(p.Factory(), recA)}
		pulProvs[i] = pulexec.Provider{
			Name: p.Name,
			Info: pulexec.BridgedProvider(t, p.Name, tfexec.Wrap(p.Factory(), recB)),
		}
	}

	var outA, outB map[string]string
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		outA = tfexec.NewDriver(t, tfProvs).Apply(t, files, c.Config)
	}()
	go func() {
		defer wg.Done()
		outB = pulexec.Run(t, pulProvs, files, c.Config).Outputs
	}()
	wg.Wait()

	if t.Failed() {
		return
	}

	require.Equal(t, outA, outB, "stack outputs differ between tofu apply and pulumi up")
	require.Equal(t, recA.Ops(), recB.Ops(), "provider operations differ between tofu apply and pulumi up")
}

// loadCase walks caseDir and returns a map of relative-path → file contents
// via loadCaseFS, failing the test on error.
func loadCase(t *testing.T, caseDir string) map[string]string {
	t.Helper()
	files, err := loadCaseFS(caseDir)
	require.NoError(t, err)
	return files
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
