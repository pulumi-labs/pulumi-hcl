package smoke_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/engine"
	"github.com/pulumi/pulumi/pkg/v3/testing/integration"
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
