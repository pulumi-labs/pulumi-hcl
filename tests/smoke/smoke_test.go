package smoke_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
	"github.com/stretchr/testify/require"
)

var langBinDir string = must(filepath.Abs(filepath.Join("..", "..", "bin")))

func must[T any](v T, err error) T {
	if err != nil {
		panic(err)
	}
	return v
}

func TestMain(m *testing.M) {
	cmd := exec.Command("make", "bin/pulumi-language-hcl")
	cmd.Dir = filepath.Dir(langBinDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building pulumi-language-hcl: %v\n", err)
		os.Exit(1)
	}

	os.Exit(m.Run())
}

func TestSmoke(t *testing.T) {
	t.Parallel()
	providerDir, err := filepath.Abs("testprovider")
	if err != nil {
		t.Fatal(err)
	}

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel: true,
		Dir:        filepath.Join("testdata", "simple"),
		Env: []string{
			"PATH=" + langBinDir + ":" + os.Getenv("PATH"),
		},
		LocalProviders: []integration.LocalDependency{
			{Package: "smoketest", Path: providerDir},
		},
		PrepareProject: func(*engine.Projinfo) error { return nil },
		Quick:          true,
		SkipRefresh:    true,
	})
}

// TestSmokeRandom proves required_providers entries pointing at a non-pulumi
// source (here, hashicorp/random ~> 3.9) are resolved through
// terraform-provider parameterization end-to-end: `pulumi install`
// materializes a local SDK, then `pulumi up` deploys against it. Hits the
// public Pulumi + TF registries.
func TestSmokeRandom(t *testing.T) {
	t.Parallel()

	pulumiBin := "pulumi"

	projectDir := copyDir(t, filepath.Join("testdata", "random"))

	stateDir := filepath.Join(projectDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	env := append(os.Environ(),
		"PATH="+langBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PULUMI_BACKEND_URL=file://"+stateDir,
		"PULUMI_CONFIG_PASSPHRASE=smoke",
	)

	runPulumi := func(args ...string) []byte {
		t.Helper()
		var stdout bytes.Buffer
		cmd := exec.Command(pulumiBin, args...)
		cmd.Dir = projectDir
		cmd.Env = env
		cmd.Stdout = io.MultiWriter(&stdout, os.Stderr)
		cmd.Stderr = os.Stderr
		require.NoErrorf(t, cmd.Run(), "pulumi %v failed", args)
		return stdout.Bytes()
	}

	runPulumi("stack", "init", "smoke-random")
	t.Cleanup(func() {
		cmd := exec.Command(pulumiBin, "stack", "rm", "--yes", "smoke-random")
		cmd.Dir = projectDir
		cmd.Env = env
		_ = cmd.Run()
	})

	runPulumi("install")

	// Assert `pulumi install` wrote the SDK descriptor where Run will look
	// for it (per docs/providers.md): sdks/random/hcl.sdk.json.
	sdkPath := filepath.Join(projectDir, "sdks", "random", "hcl.sdk.json")
	_, err := os.Stat(sdkPath)
	require.NoErrorf(t, err, "pulumi install must write %s", sdkPath)

	runPulumi("up", "--yes", "--skip-preview")

	// `length = 2` produces "<word>-<word>" (lowercase letters only).
	outputs := parseStackOutput(t, runPulumi("stack", "output", "--json"))
	pet, ok := outputs["pet"].(string)
	require.True(t, ok, "pet output must be a string, got %T (%v)", outputs["pet"], outputs["pet"])
	require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+$`), pet,
		"pet output should match '<word>-<word>'")

	runPulumi("destroy", "--yes", "--skip-preview")
}

// TestSmokeModule proves an HCL module declaring a `component {}` block can be
// served as a Multi-Language Component: `pulumi package add ../random-module`
// generates a local SDK, the consuming HCL program instantiates the component
// (passing `length` in, reading `pet` out), the component's `random_pet` (a
// bridged hashicorp/random resource) is created via Construct, and the root
// output is unknown at preview and a three-word pet name after up.
func TestSmokeModule(t *testing.T) {
	t.Parallel()

	pulumiBin := "pulumi"

	rootDir := copyTree(t, filepath.Join("testdata", "module"))
	projectDir := filepath.Join(rootDir, "program")

	stateDir := filepath.Join(rootDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	env := append(os.Environ(),
		"PATH="+langBinDir+string(os.PathListSeparator)+os.Getenv("PATH"),
		"PULUMI_BACKEND_URL=file://"+stateDir,
		"PULUMI_CONFIG_PASSPHRASE=smoke",
	)

	runPulumi := func(t *testing.T, args ...string) []byte {
		t.Helper()
		var stdout bytes.Buffer
		cmd := exec.CommandContext(t.Context(), pulumiBin, args...)
		cmd.Dir = projectDir
		cmd.Env = env
		cmd.Stdout = io.MultiWriter(&stdout, os.Stderr)
		cmd.Stderr = os.Stderr
		require.NoErrorf(t, cmd.Run(), "pulumi %v failed", args)
		return stdout.Bytes()
	}

	runPulumi(t, "stack", "init", "smoke-module")
	t.Cleanup(func() {
		cmd := exec.Command(pulumiBin, "stack", "rm", "--yes", "smoke-module")
		cmd.Dir = projectDir
		cmd.Env = env
		_ = cmd.Run()
	})

	runPulumi(t, "package", "add", "../random-module")

	// The pet name flows from a not-yet-created `random_pet` inside the
	// component, so the root output is unknown during preview.
	require.Regexp(t, regexp.MustCompile(`pet: \[unknown\]`), string(runPulumi(t, "preview")),
		"pet output should be unknown at preview")

	runPulumi(t, "up", "--yes", "--skip-preview")

	// `length = 3` produces "<word>-<word>-<word>" (lowercase letters only).
	outputs := parseStackOutput(t, runPulumi(t, "stack", "output", "--json"))
	pet, ok := outputs["pet"].(string)
	require.True(t, ok, "pet output must be a string, got %T (%v)", outputs["pet"], outputs["pet"])
	require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`), pet,
		"pet output should match '<word>-<word>-<word>'")

	runPulumi(t, "destroy", "--yes", "--skip-preview")
}

// parseStackOutput parses `pulumi stack output --json` (a JSON object).
func parseStackOutput(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out), "parsing stack output: %s", raw)
	return out
}

// copyDir copies src into a fresh t.TempDir() subdir and returns the new path.
// Used so `pulumi install` can mutate Pulumi.yaml without dirtying testdata.
func copyDir(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	entries, err := os.ReadDir(src)
	require.NoError(t, err)
	for _, e := range entries {
		require.False(t, e.IsDir(), "nested dirs not supported")
		copyFile(t, filepath.Join(src, e.Name()), filepath.Join(dst, e.Name()))
	}
	return dst
}

// copyTree recursively copies src into a fresh t.TempDir() and returns the new
// path. Used for fixtures with subdirectories (e.g. a component module alongside
// its consuming program) that `pulumi package add`/`install` mutate.
func copyTree(t *testing.T, src string) string {
	t.Helper()
	dst := t.TempDir()
	require.NoError(t, filepath.WalkDir(src, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		target := filepath.Join(dst, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		copyFile(t, path, target)
		return nil
	}))
	return dst
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	require.NoError(t, err)
	defer func() { _ = in.Close() }()
	out, err := os.Create(dst)
	require.NoError(t, err)
	defer func() { _ = out.Close() }()
	_, err = io.Copy(out, in)
	require.NoError(t, err)
}
