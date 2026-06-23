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
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/stretchr/testify/require"
)

func TestMain(m *testing.M) {
	langBinDir, err := filepath.Abs(filepath.Join("..", "..", "bin"))
	if err != nil {
		fmt.Fprintf(os.Stderr, "finding plugin dir: %v\n", err)
		os.Exit(1)
	}
	cmd := exec.Command("make", "bin/pulumi-language-hcl", "bin/pulumi-resource-hcl")
	cmd.Dir = filepath.Dir(langBinDir)
	cmd.Stdout = os.Stderr
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Fprintf(os.Stderr, "building plugins: %v\n", err)
		os.Exit(1)
	}

	os.Setenv("PATH", langBinDir+string(os.PathListSeparator)+os.Getenv("PATH"))

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

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel:     true,
		Dir:            filepath.Join("testdata", "random"),
		PrepareProject: prepareWithPulumiInstall,
		PostPrepareProject: func(e *engine.Projinfo) error {
			sdkPath := filepath.Join(e.Root, "sdks", "random", "hcl.sdk.json")
			_, err := os.Stat(sdkPath)
			return err
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			pet, ok := stack.Outputs["pet"].(string)
			require.True(t, ok, "pet output must be a string, got %T (%v)",
				stack.Outputs["pet"], stack.Outputs["pet"])
			require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+$`), pet,
				"pet output should match '<word>-<word>'")

		},
	})
}

// TestSmokeModule proves a plain HCL module (no component or package block) can
// be served as a Multi-Language Component: `pulumi package add ../randommodule`
// generates a local SDK, the consuming HCL program instantiates the component by
// its bare package name (passing `length` in, reading `pet` out), the component's
// `random_pet` (a bridged hashicorp/random resource) is created via Construct,
// and the root output is unknown at preview and a three-word pet name after up.
func TestSmokeModule(t *testing.T) {
	t.Parallel()

	pulumiBin := "pulumi"

	rootDir := copyTree(t, filepath.Join("testdata", "module"))
	projectDir := filepath.Join(rootDir, "program")

	stateDir := filepath.Join(rootDir, "state")
	require.NoError(t, os.MkdirAll(stateDir, 0o755))

	env := append(os.Environ(),
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

	runPulumi(t, "package", "add", "../randommodule")

	// The pet name flows from a not-yet-created `random_pet` inside the
	// component, so that output is unknown during preview.
	require.Regexp(t, regexp.MustCompile(`pet_name\s*: \[unknown\]`), string(runPulumi(t, "preview")),
		"pet_name output should be unknown at preview")

	runPulumi(t, "up", "--yes", "--skip-preview")

	// The multi-word `pet_length`, nested object field `string_field`, and map
	// value `user_key` all round-trip through the component, which only works if
	// the boundary translates object/field names (but not map keys) in both
	// directions: a snake_case HCL module exposed under a camelCase schema.
	outputs := parseStackOutput(t, runPulumi(t, "stack", "output", "--json"))

	petName, ok := outputs["pet_name"].(string)
	require.True(t, ok, "pet_name must be a string, got %T (%v)", outputs["pet_name"], outputs["pet_name"])
	require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`), petName,
		"pet_length = 3 should yield a three-word pet name")

	require.Equal(t, "hello", outputs["object_field"], "object field value should round-trip")
	require.Equal(t, "world", outputs["map_field"], "map value (keyed by a preserved key) should round-trip")

	runPulumi(t, "destroy", "--yes", "--skip-preview")
}

// TestSmokeDynamicModule exercises the fully dynamic MLC: a YAML program
// instantiates the untyped hcl:index:Module resource (served by
// pulumi-resource-hcl) with a module source supplied as a plain input. The
// module references a native Pulumi provider (pulumi/random) and a bridged
// Terraform provider (hashicorp/null); both are resolved at Construct time
// through the schema loader, mapper, and resolver the engine exposes at
// handshake. Hits the public Pulumi + TF registries.
func TestSmokeDynamicModule(t *testing.T) {
	t.Parallel()

	// The module source is a plain input known before Construct. LoadModule only
	// parses a local source, so the fixture can be referenced in place by absolute
	// path rather than copied.
	moduleDir, err := filepath.Abs(filepath.Join("testdata", "dynamic", "module"))
	require.NoError(t, err)

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel: true,
		Dir:        filepath.Join("testdata", "dynamic", "program"),
		Config:     map[string]string{"moduleSource": moduleDir},
		Quick:      true, // TODO: undo
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			// The module's `name` output is "<prefix>-<random_string>"; its presence
			// and shape prove both providers resolved and the outputs flowed back
			// through the component's untyped `outputs` map.
			name, ok := stack.Outputs["name"].(string)
			require.True(t, ok, "name output must be a string, got %T (%v)",
				stack.Outputs["name"], stack.Outputs["name"])
			require.Regexp(t, regexp.MustCompile(`^smoke-[a-z0-9]{8}$`), name,
				"name should be '<prefix>-<8 lowercase alphanumerics>'")
		},
	})
}

// parseStackOutput parses `pulumi stack output --json` (a JSON object).
func parseStackOutput(t *testing.T, raw []byte) map[string]any {
	t.Helper()
	var out map[string]any
	require.NoError(t, json.Unmarshal(raw, &out), "parsing stack output: %s", raw)
	return out
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
	defer contract.IgnoreClose(in)
	out, err := os.Create(dst)
	require.NoError(t, err)
	defer contract.IgnoreClose(out)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
}

func prepareWithPulumiInstall(e *engine.Projinfo) error {
	cmd := exec.Command("pulumi", "install")
	cmd.Dir = e.Root
	return cmd.Run()
}
