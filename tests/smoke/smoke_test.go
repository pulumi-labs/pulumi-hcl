package smoke_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
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

	if err := os.Setenv("PATH", langBinDir+string(os.PathListSeparator)+os.Getenv("PATH")); err != nil {
		fmt.Fprintf(os.Stderr, "setting PATH: %v\n", err)
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

	home := seedPluginCache(t, "terraform-provider")
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel:     true,
		Dir:            filepath.Join("testdata", "random"),
		PulumiHomeDir:  home,
		PrepareProject: prepareWithPulumiInstall(home),
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

// TestSmokeInstallConverges reproduces the `pulumi install` loop hit when two
// modules resolve one provider through different spellings: the root spells
// the source fully qualified while the child module requires the same
// provider implicitly ("hashicorp/random"). Requirements used to be tracked
// per spelling but satisfaction was cleared only for the root's, so the
// child's spec was re-emitted forever and every preview demanded another
// `pulumi install`. One install must converge and the program must deploy.
func TestSmokeInstallConverges(t *testing.T) {
	t.Parallel()

	home := seedPluginCache(t, "terraform-provider")
	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel:     true,
		Dir:            filepath.Join("testdata", "install-loop"),
		PulumiHomeDir:  home,
		PrepareProject: prepareWithPulumiInstall(home),
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			pet, ok := stack.Outputs["pet"].(string)
			require.True(t, ok, "pet output must be a string, got %T (%v)",
				stack.Outputs["pet"], stack.Outputs["pet"])
			require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+$`), pet,
				"pet output should match '<word>-<word>'")
		},
	})
}

// TestSmokeInLanguageModule proves a plain HCL module (no component or package block) can
// be served as a Multi-Language Component: `pulumi package add ../randommodule` generates
// a local SDK, the consuming HCL program instantiates the component as
// `randommodule_module` (passing `length` in, reading `pet` out), the component's
// `random_pet` (a bridged hashicorp/random resource) is created via Construct, and the
// root output is unknown at preview and a three-word pet name after up.
func TestSmokeInLanguageModule(t *testing.T) {
	t.Parallel()

	home := seedPluginCache(t, "terraform-provider", "local")

	// `pulumi package add` generates the SDK from a local module; the program
	// references the component by the module's package name, "randommodule". The
	// program (which package add and up mutate) is the copied ProgramTest dir, but
	// the module lives outside it, so copy it under its package name and add it by
	// absolute path.
	moduleDir := filepath.Join(t.TempDir(), "randommodule")
	copyDir(t, filepath.Join("testdata", "module", "randommodule"), moduleDir)

	// ProgramTest runs a `pulumi preview` before the update; with Verbose its
	// stdout is captured here so the validation can assert the component output is
	// unknown at preview.
	var preview bytes.Buffer

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel:    true,
		Dir:           filepath.Join("testdata", "module", "program"),
		PulumiHomeDir: home,
		Verbose:       true,
		Stdout:        &preview,
		PrepareProject: func(e *engine.Projinfo) error {
			cmd := exec.Command("pulumi", "package", "add", moduleDir)
			cmd.Dir = e.Root
			cmd.Env = append(os.Environ(), "PULUMI_HOME="+home)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			return cmd.Run()
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			// The pet name flows from a not-yet-created `random_pet` inside the
			// component, so the component output is unknown during preview.
			require.Regexp(t, regexp.MustCompile(`pet_name\s*:\s*\[unknown\]`), preview.String(),
				"pet_name output should be unknown at preview")

			// The multi-word `pet_length`, nested object field `string_field`, and
			// map value `user_key` all round-trip through the component, which only
			// works if the boundary translates object/field names (but not map keys)
			// in both directions: a snake_case HCL module exposed under a camelCase
			// schema.
			petName, ok := stack.Outputs["pet_name"].(string)
			require.True(t, ok, "pet_name must be a string, got %T (%v)",
				stack.Outputs["pet_name"], stack.Outputs["pet_name"])
			require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`), petName,
				"pet_length = 3 should yield a three-word pet name")

			require.Equal(t, "hello", stack.Outputs["object_field"], "object field value should round-trip")
			require.Equal(t, "world", stack.Outputs["map_field"], "map value (keyed by a preserved key) should round-trip")

			require.Equal(t, "1.2.3", stack.Outputs["module_version"],
				`module_version should be read from "${path.module}/VERSION"`)
		},
	})
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
		NoParallel:    true,
		Dir:           filepath.Join("testdata", "dynamic", "program"),
		Config:        map[string]string{"moduleSource": moduleDir},
		PulumiHomeDir: seedPluginCache(t, "random", "terraform-provider", "local"),
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			// The module's `name` output is "<prefix>-<random_string>"; its presence
			// and shape prove both providers resolved and the outputs flowed back
			// through the component's untyped `outputs` map.
			name, ok := stack.Outputs["name"].(string)
			require.True(t, ok, "name output must be a string, got %T (%v)",
				stack.Outputs["name"], stack.Outputs["name"])
			require.Regexp(t, regexp.MustCompile(`^smoke-[a-z0-9]{8}$`), name,
				"name should be '<prefix>-<8 lowercase alphanumerics>'")

			require.Equal(t, "4.5.6", stack.Outputs["moduleVersion"],
				`moduleVersion should be read from "${path.module}/VERSION"`)
		},
	})
}

// TestSmokeParameterizedModule exercises the parameterization path through
// pulumi-resource-hcl: `pulumi package add hcl module <source>` downloads and
// bundles the same module TestSmokeInLanguageModule serves as a local-path MLC, generates
// its typed component SDK, and a YAML program instantiates that component by its
// token (`randommodule:index:Module`), passing `petLength` in and reading
// `petName` out. The module's `random_pet` (a bridged hashicorp/random resource)
// is resolved through terraform-provider parameterization, so this drives the
// whole module-bundle-then-consume flow end-to-end. Hits the public Pulumi + TF
// registries.
func TestSmokeParameterizedModule(t *testing.T) {
	t.Parallel()

	home := seedPluginCache(t, "terraform-provider", "local")

	// The same fixture TestSmokeInLanguageModule serves as a local-path MLC; here it is
	// loaded by path and bundled into the parameterization, leaving the program
	// dir untouched, so the fixture can be referenced in place by absolute path.
	moduleDir, err := filepath.Abs(filepath.Join("testdata", "module", "randommodule"))
	require.NoError(t, err)

	// `pulumi package add` resolves a bare plugin name (`hcl`) through the
	// registry, which the locally-built plugin is not in; pointing it at the
	// built binary on PATH parameterizes the plugin in place.
	resourceBin, err := filepath.Abs(filepath.Join("..", "..", "bin", "pulumi-resource-hcl"))
	require.NoError(t, err)

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel:    true,
		Dir:           filepath.Join("testdata", "parameterized", "program"),
		PulumiHomeDir: home,
		PrepareProject: func(e *engine.Projinfo) error {
			cmd := exec.Command("pulumi", "package", "add", resourceBin, "module", moduleDir)
			cmd.Dir = e.Root
			cmd.Env = append(os.Environ(), "PULUMI_HOME="+home)
			cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
			return cmd.Run()
		},
		ExtraRuntimeValidation: func(t *testing.T, stack integration.RuntimeValidationStackInfo) {
			// petLength = 3 flows into the module's random_pet through the
			// camelCase->snake_case boundary, and the resulting three-word pet name
			// flows back out, proving the parameterized component round-trips inputs
			// and outputs.
			petName, ok := stack.Outputs["petName"].(string)
			require.True(t, ok, "petName must be a string, got %T (%v)",
				stack.Outputs["petName"], stack.Outputs["petName"])
			require.Regexp(t, regexp.MustCompile(`^[a-z]+-[a-z]+-[a-z]+$`), petName,
				"petLength = 3 should yield a three-word pet name")

			require.Equal(t, "1.2.3", stack.Outputs["moduleVersion"],
				`moduleVersion should be read from "${path.module}/VERSION"`)
		},
	})
}

// TestSmokeDestroyHooks proves resource-hook-backed constructs (precondition,
// postcondition, create and destroy provisioners) work end-to-end through the
// dynamic MLC. `up` runs the create provisioner and `destroy --run-program`
// fires the destroy provisioner; each drops a marker file the test asserts on.
func TestSmokeDestroyHooks(t *testing.T) {
	t.Parallel()

	moduleDir, err := filepath.Abs(filepath.Join("testdata", "destroy-hooks", "module"))
	require.NoError(t, err)
	markerDir := t.TempDir()

	integration.ProgramTest(t, &integration.ProgramTestOptions{
		NoParallel: true,
		Dir:        filepath.Join("testdata", "destroy-hooks", "program"),
		Config: map[string]string{
			"moduleSource": moduleDir,
			"markerDir":    markerDir,
		},
		Quick:       true,
		SkipRefresh: true,
		// The destroy-time provisioner is a before_delete hook; the engine only
		// runs it when destroy re-runs the program to re-register it.
		DestroyCommandlineFlags: []string{"--run-program"},
		ExtraRuntimeValidation: func(t *testing.T, _ integration.RuntimeValidationStackInfo) {
			require.FileExists(t, filepath.Join(markerDir, "created"),
				"create-time provisioner should have run during up")
		},
	})

	// ProgramTest runs up (validated above) then destroy --run-program; once it
	// returns, the destroy-time provisioner must have left its marker.
	require.FileExists(t, filepath.Join(markerDir, "destroyed"),
		"destroy-time provisioner should have run during destroy --run-program")
}

func copyDir(t *testing.T, src, dst string) {
	t.Helper()
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
}

func copyFile(t *testing.T, src, dst string) {
	t.Helper()
	info, err := os.Stat(src)
	require.NoError(t, err)
	in, err := os.Open(src)
	require.NoError(t, err)
	defer contract.IgnoreClose(in)
	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, info.Mode())
	require.NoError(t, err)
	defer contract.IgnoreClose(out)
	_, err = io.Copy(out, in)
	require.NoError(t, err)
}

func seedPluginCache(t *testing.T, plugins ...string) string {
	t.Helper()
	home := t.TempDir()
	entries, err := os.ReadDir(hostPluginsDir())
	if err != nil {
		return home // No host cache to seed from; the test downloads as usual.
	}
	dst := filepath.Join(home, "plugins")
	require.NoError(t, os.MkdirAll(dst, 0o755))
	for _, e := range entries {
		for _, name := range plugins {
			if !strings.HasPrefix(e.Name(), "resource-"+name+"-v") {
				continue
			}
			from, to := filepath.Join(hostPluginsDir(), e.Name()), filepath.Join(dst, e.Name())
			if e.IsDir() {
				copyDir(t, from, to)
			} else {
				copyFile(t, from, to)
			}
		}
	}
	return home
}

func hostPluginsDir() string {
	if h := os.Getenv("PULUMI_HOME"); h != "" {
		return filepath.Join(h, "plugins")
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(home, ".pulumi", "plugins")
}

func prepareWithPulumiInstall(pulumiHome string) func(*engine.Projinfo) error {
	return func(e *engine.Projinfo) error {
		cmd := exec.Command("pulumi", "install")
		cmd.Dir = e.Root
		cmd.Env = append(os.Environ(), "PULUMI_HOME="+pulumiHome)
		return cmd.Run()
	}
}
