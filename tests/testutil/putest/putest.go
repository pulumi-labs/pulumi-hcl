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

// Package putest is the Pulumi-only half of the tfcompat harness: it runs a
// `.tf` program from testdata/cases/<name>/ through `pulumi up` with no
// OpenTofu run to compare against; assertions are made directly against stack
// outputs, exported Pulumi state, and the recorded provider operations. Use
// it for setups a tf-compatible program cannot produce — provider-info
// customization (Customize) foremost — and, with Provider.Dynamic, to lock in
// the shipping dynamic bridge's behavior on cases skipped in tfcompat as
// known divergences from OpenTofu. Everything else that a real tf-compatible
// program can produce belongs in the tfcompat harness instead, where OpenTofu
// itself defines the expected behavior.
package putest

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strconv"
	"testing"

	pfprovider "github.com/hashicorp/terraform-plugin-framework/provider"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/pulumi/pulumi-hcl/tests/testutil/pulexec"
	"github.com/pulumi/pulumi-hcl/tests/testutil/tfexec"
	"github.com/pulumi/pulumi-terraform-bridge/v3/pkg/tfbridge"
	"github.com/pulumi/pulumi/sdk/v3/go/common/apitype"
	"github.com/stretchr/testify/require"
)

// Provider describes an in-memory TF provider bridged in-process for the
// Pulumi path.
type Provider struct {
	Name string
	// Factory builds an SDKv2 (helper/schema) provider. Exactly one of
	// Factory and PFFactory must be set.
	Factory func() *schema.Provider
	// PFFactory builds a terraform-plugin-framework provider.
	PFFactory func() pfprovider.Provider
	// Customize, if non-nil, runs against the bridged ProviderInfo so tests
	// can apply non-default Pulumi-side renames (or any other ProviderInfo
	// tweak) to exercise the bridge mapping behaviour.
	Customize func(*testing.T, *tfbridge.ProviderInfo)
	// Dynamic serves the provider through the real terraform-provider plugin
	// via reattach (the same path tfcompat's pulumi side uses) instead of the
	// in-process bridge. Use it to lock in behavior of the shipping dynamic
	// bridge — e.g. cases skipped in tfcompat as known divergences from
	// OpenTofu. Incompatible with Customize, which only exists in-process.
	Dynamic bool
}

// Case is the test description passed to RunCase.
type Case struct {
	Providers []Provider
	// Config is set as stack config.
	Config map[string]string
	// ExpectErr, if non-empty, requires the last apply to fail with an error
	// containing this substring; the other assertions are skipped.
	ExpectErr string
	// ExpectedOutputs, if non-nil, must equal the stack outputs exactly.
	// Non-string outputs appear in their compact-JSON form (see
	// pulexec.Result).
	ExpectedOutputs map[string]string
	// AssertState, if set, runs against the exported resources after the
	// apply.
	AssertState func(t *testing.T, resources []apitype.ResourceV3)
	// AssertOps, if set, runs against the recorded provider operations after
	// the apply. SDKv2 providers record at the helper/schema CRUD boundary,
	// plugin-framework providers at the tfprotov6 boundary.
	AssertOps func(t *testing.T, ops []tfexec.Op)
}

// RunCase resolves testdata/cases/<caseName>/ relative to the calling test
// file, runs `pulumi up` on it, and applies the Case's assertions. A case
// directory containing only numbered stage subdirs (0/, 1/, ...) applies one
// file set per subdir in order; assertions run after the last apply.
func RunCase(t *testing.T, caseName string, c Case) {
	t.Helper()

	_, callerFile, _, _ := runtime.Caller(1)
	caseDir := filepath.Join(filepath.Dir(callerFile), "testdata", "cases", caseName)
	stages, err := loadStages(caseDir)
	require.NoError(t, err)

	rec := &tfexec.Recorder{}
	provs := make([]pulexec.Provider, len(c.Providers))
	for i, p := range c.Providers {
		if p.Dynamic && p.Customize != nil {
			t.Fatalf("provider %q: Dynamic is incompatible with Customize", p.Name)
		}
		switch {
		case p.Factory != nil && p.PFFactory == nil:
			factory := p.Factory
			wrapped := func() *schema.Provider { return tfexec.Wrap(factory(), rec) }
			if p.Dynamic {
				provs[i] = pulexec.SDKv2ProviderDynamic(t, p.Name, wrapped)
			} else {
				provs[i] = pulexec.SDKv2Provider(t, p.Name, wrapped, p.Customize)
			}
		case p.PFFactory != nil && p.Factory == nil:
			if p.Dynamic {
				provs[i] = pulexec.PFProviderDynamic(t, p.Name, p.PFFactory, rec)
			} else {
				provs[i] = pulexec.PFProvider(t, p.Name, p.PFFactory, rec, p.Customize)
			}
		default:
			t.Fatalf("provider %q: exactly one of Factory or PFFactory must be set", p.Name)
		}
	}

	driver := pulexec.NewDriver(t, provs, c.Config)
	var res pulexec.Result
	for i, files := range stages {
		res, err = driver.TryApply(t, files)
		if i < len(stages)-1 || c.ExpectErr == "" {
			require.NoErrorf(t, err, "stage %d: pulumi up failed", i)
		}
	}

	if c.ExpectErr != "" {
		require.Error(t, err, "pulumi up was expected to fail with %q", c.ExpectErr)
		require.Contains(t, err.Error(), c.ExpectErr)
		return
	}
	if c.ExpectedOutputs != nil {
		require.Equal(t, c.ExpectedOutputs, res.Outputs)
	}
	if c.AssertState != nil {
		c.AssertState(t, res.Resources)
	}
	if c.AssertOps != nil {
		c.AssertOps(t, rec.Ops())
	}
}

// loadStages returns one file set per stage: a case directory containing only
// numbered subdirs (0/, 1/, ...) yields that many file sets in order; any
// other shape yields the whole directory as a single stage.
func loadStages(caseDir string) ([]map[string]string, error) {
	entries, err := os.ReadDir(caseDir)
	if err != nil {
		return nil, fmt.Errorf("case directory: %w", err)
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
		files, err := loadCaseDir(caseDir)
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
	fileSets := make([]map[string]string, 0, len(keys))
	for _, k := range keys {
		files, err := loadCaseDir(stageDirs[k])
		if err != nil {
			return nil, fmt.Errorf("stage %d: %w", k, err)
		}
		fileSets = append(fileSets, files)
	}
	return fileSets, nil
}

// loadCaseDir reads every regular file under caseDir and returns a map of
// relative-path → file contents.
func loadCaseDir(caseDir string) (map[string]string, error) {
	files := make(map[string]string)
	err := filepath.WalkDir(caseDir, func(path string, d os.DirEntry, err error) error {
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
