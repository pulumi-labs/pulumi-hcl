package smoke_test

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
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
// source (here, hashicorp/random) are resolved through terraform-provider
// parameterization end-to-end: `pulumi install` materializes a local SDK,
// then `pulumi up` deploys against it. Hits the public Pulumi + TF registries.
//
// Requires a pulumi CLI that includes pulumi/pulumi#23330 (package-spec install
// support). Set PULUMI_BIN to the path of such a binary to enable the test.
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

	runPulumi := func(args ...string) {
		t.Helper()
		cmd := exec.Command(pulumiBin, args...)
		cmd.Dir = projectDir
		cmd.Env = env
		cmd.Stdout = os.Stderr
		cmd.Stderr = os.Stderr
		require.NoErrorf(t, cmd.Run(), "pulumi %v failed", args)
	}

	runPulumi("stack", "init", "smoke-random")
	t.Cleanup(func() {
		cmd := exec.Command(pulumiBin, "stack", "rm", "--yes", "smoke-random")
		cmd.Dir = projectDir
		cmd.Env = env
		_ = cmd.Run()
	})

	runPulumi("install")
	runPulumi("up", "--yes", "--skip-preview")
	runPulumi("destroy", "--yes", "--skip-preview")
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

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	in, err := os.Open(src)
	require.NoError(t, err)
	defer in.Close()
	out, err := os.Create(dst)
	require.NoError(t, err)
	defer out.Close()
	_, err = io.Copy(out, in)
	require.NoError(t, err)
}
