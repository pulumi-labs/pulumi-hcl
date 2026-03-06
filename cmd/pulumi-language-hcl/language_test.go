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

// Adding a new language test
//
// Language tests are defined in the pulumi/pulumi repo under
// pkg/testing/pulumi-test-language/tests/. Each test has a Go file
// defining the test configuration and a testdata directory containing
// one or more .pp (PCL) source files.
//
// To enable a new language test for the HCL runtime:
//
//  1. Look up the test's .pp files in the pulumi repo to understand what
//     PCL constructs the test uses (outputs, resources, function calls, etc.).
//
//  2. Ensure the HCL codegen (pkg/codegen/generate.go) can convert the PCL
//     constructs to valid HCL. Add support for any missing expression types,
//     function calls, or resource types.
//
//  3. Ensure the HCL runtime (pkg/hcl/run/, pkg/hcl/eval/, pkg/server/)
//     can execute the generated HCL correctly. This may involve adding
//     support for new built-in resource types, functions, or path handling.
//
//  4. Remove the test name from the expectedFailures map below.
//
//  5. Run the test with PULUMI_ACCEPT=1 to generate snapshot files:
//     PULUMI_ACCEPT=1 go test ./cmd/pulumi-language-hcl/ -run 'TestLanguage/<test-name>' -count=1 -v
//     This creates/updates files under testdata/projects/<test-name>/.
//
//  6. Run the test without PULUMI_ACCEPT to verify it passes:
//     go test ./cmd/pulumi-language-hcl/ -run 'TestLanguage/<test-name>' -count=1 -v

package main

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"sync"
	"testing"

	"github.com/pulumi/pulumi-language-hcl/pkg/converter"
	"github.com/pulumi/pulumi-language-hcl/pkg/server"

	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/rpcutil"
	pulumirpc "github.com/pulumi/pulumi/sdk/v3/proto/go"
	testingrpc "github.com/pulumi/pulumi/sdk/v3/proto/go/testing"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
)

func runTestingHost(t *testing.T) (string, testingrpc.LanguageTestClient) {
	// We can't just go run the pulumi-test-language package because of
	// https://github.com/golang/go/issues/39172, so we build it to a temp file then run that.
	binary := t.TempDir() + "/pulumi-test-language"
	cmd := exec.CommandContext(t.Context(),
		"go", "build", "-o", binary,
		"github.com/pulumi/pulumi/pkg/v3/testing/pulumi-test-language")
	output, err := cmd.CombinedOutput()
	if len(output) > 0 {
		t.Logf("build output: %s", output)
	}
	require.NoError(t, err)

	cmd = exec.CommandContext(t.Context(), binary)
	stdout, err := cmd.StdoutPipe()
	require.NoError(t, err)
	stderr, err := cmd.StderrPipe()
	require.NoError(t, err)
	stderrReader := bufio.NewReader(stderr)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for {
			text, err := stderrReader.ReadString('\n')
			if err != nil {
				wg.Done()
				return
			}
			t.Logf("engine: %s", text)
		}
	}()

	err = cmd.Start()
	require.NoError(t, err)

	stdoutBytes, err := io.ReadAll(stdout)
	require.NoError(t, err)

	address := string(stdoutBytes)

	conn, err := grpc.NewClient(
		address,
		grpc.WithTransportCredentials(insecure.NewCredentials()),
		grpc.WithUnaryInterceptor(rpcutil.OpenTracingClientInterceptor()),
		grpc.WithStreamInterceptor(rpcutil.OpenTracingStreamClientInterceptor()),
		rpcutil.GrpcChannelOptions(),
	)
	require.NoError(t, err)

	client := testingrpc.NewLanguageTestClient(conn)

	t.Cleanup(func() {
		assert.NoError(t, cmd.Process.Kill())
		wg.Wait()
		// We expect this to error because we just killed it.
		contract.IgnoreError(cmd.Wait())
	})

	return address, client
}

var expectedFailures = map[string]string{
	"l2-plain": "unsupported in HCL:" +
		" requires that HCL can distinguish between an empty and null List<Object>" +
		" - not compatible with block syntax",
}

// expectedEjectFailures lists tests whose eject (HCL→PCL conversion) step is
// expected to fail because the converter does not yet support resources, data
// sources, or other constructs used by those tests.
var expectedEjectFailures = map[string]string{
	"l2-invoke-scalar":                             "converter does not support resource/data/call blocks",
	"l2-invoke-scalars":                            "converter does not support resource/data/call blocks",
	"l2-invoke-secrets":                            "converter does not support resource/data/call blocks",
	"l2-invoke-simple":                             "converter does not support resource/data/call blocks",
	"l2-invoke-variants":                           "converter does not support resource/data/call blocks",
	"l2-keywords":                                  "converter does not support resource/data/call blocks",
	"l2-large-string":                              "converter does not support resource/data/call blocks",
	"l2-map-keys":                                  "converter does not support resource/data/call blocks",
	"l2-module-format":                             "converter does not support resource/data/call blocks",
	"l2-namespaced-provider":                       "converter does not support resource/data/call blocks",
	"l2-parallel-resources":                        "converter does not support resource/data/call blocks",
	"l2-parameterized-invoke":                      "converter does not support resource/data/call blocks",
	"l2-parameterized-resource":                    "converter does not support resource/data/call blocks",
	"l2-parameterized-resource-twice":              "converter does not support resource/data/call blocks",
	"l2-plain":                                     "converter does not support resource/data/call blocks",
	"l2-primitive-ref":                             "converter does not support resource/data/call blocks",
	"l2-provider-call":                             "converter does not support resource/data/call blocks",
	"l2-provider-call-explicit":                    "converter does not support resource/data/call blocks",
	"l2-provider-grpc-config":                      "converter does not support resource/data/call blocks",
	"l2-provider-grpc-config-schema-secret":        "converter does not support resource/data/call blocks",
	"l2-provider-grpc-config-secret":               "converter does not support resource/data/call blocks",
	"l2-proxy-index":                               "converter does not support resource/data/call blocks",
	"l2-ref-ref":                                   "converter does not support resource/data/call blocks",
	"l2-resource-alpha":                            "converter does not support resource/data/call blocks",
	"l2-resource-asset-archive":                    "converter does not support resource/data/call blocks",
	"l2-resource-config":                           "converter does not support resource/data/call blocks",
	"l2-resource-default":                          "converter does not support resource/data/call blocks",
	"l2-resource-deletion-before-replacement":      "converter does not support resource/data/call blocks",
	"l2-resource-destroy":                          "converter does not support resource/data/call blocks",
	"l2-resource-elide-unknowns":                   "converter does not support resource/data/call blocks",
	"l2-resource-invoke":                           "converter does not support resource/data/call blocks",
	"l2-resource-invoke-component":                 "converter does not support resource/data/call blocks",
	"l2-resource-invoke-dynamic-function":          "converter does not support resource/data/call blocks",
	"l2-resource-keyword-overlap":                  "converter does not support resource/data/call blocks",
	"l2-resource-methods":                          "converter does not support resource/data/call blocks",
	"l2-resource-name-type":                        "converter does not support resource/data/call blocks",
	"l2-resource-names":                            "converter does not support resource/data/call blocks",
	"l2-resource-option-additional-secret-outputs": "converter does not support resource/data/call blocks",
	"l2-resource-option-alias":                     "converter does not support resource/data/call blocks",
	"l2-resource-option-custom-timeouts":           "converter does not support resource/data/call blocks",
	"l2-resource-option-delete-before-replace":     "converter does not support resource/data/call blocks",
	"l2-resource-option-deleted-with":              "converter does not support resource/data/call blocks",
	"l2-resource-option-depends-on":                "converter does not support resource/data/call blocks",
	"l2-resource-option-env-var-mappings":          "converter does not support resource/data/call blocks",
	"l2-resource-option-hide-diffs":                "converter does not support resource/data/call blocks",
	"l2-resource-option-ignore-changes":            "converter does not support resource/data/call blocks",
	"l2-resource-option-import":                    "converter does not support resource/data/call blocks",
	"l2-resource-option-plugin-download-url":       "converter does not support resource/data/call blocks",
	"l2-resource-option-protect":                   "converter does not support resource/data/call blocks",
	"l2-resource-option-replace-on-changes":        "converter does not support resource/data/call blocks",
	"l2-resource-option-replace-with":              "converter does not support resource/data/call blocks",
	"l2-resource-option-replacement-trigger":       "converter does not support resource/data/call blocks",
	"l2-resource-option-retain-on-delete":          "converter does not support resource/data/call blocks",
	"l2-resource-option-version":                   "converter does not support resource/data/call blocks",
	"l2-resource-option-version-sdk":               "converter does not support resource/data/call blocks",
	"l2-resource-options":                          "converter does not support resource/data/call blocks",
	"l2-resource-order":                            "converter does not support resource/data/call blocks",
	"l2-resource-parent":                           "converter does not support resource/data/call blocks",
	"l2-resource-parent-inheritance":               "converter does not support resource/data/call blocks",
	"l2-resource-primitives":                       "converter does not support resource/data/call blocks",
	"l2-resource-provider-call":                    "converter does not support resource/data/call blocks",
	"l2-resource-ref":                              "converter does not support resource/data/call blocks",
	"l2-resource-secret":                           "converter does not support resource/data/call blocks",
	"l2-resource-simple":                           "converter does not support resource/data/call blocks",
	"l2-resource-with-alone":                       "converter does not support resource/data/call blocks",
	"l2-target-up-with-new-dependency":             "converter does not support resource/data/call blocks",
	"l2-union":                                     "converter does not support resource/data/call blocks",
	"l3-component-call":                            "converter does not support resource/data/call blocks",
	"l3-component-simple":                          "converter does not support resource/data/call blocks",
	"l3-range":                                     "converter does not support resource/data/call blocks",
	"l3-range-resource-output-traversal":           "converter does not support resource/data/call blocks",
	"l3-resource-simple":                           "converter does not support resource/data/call blocks",
}

func has[K comparable, V any, M ~map[K]V](m M, k K) bool {
	_, ok := m[k]
	return ok
}

func log(t *testing.T, name, message string) {
	if os.Getenv("PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT") != "true" {
		if len(message) > 1024 {
			message = message[:1024] + "... (truncated, run with PULUMI_LANGUAGE_TEST_SHOW_FULL_OUTPUT=true to see full logs))"
		}
	}
	t.Logf("%s: %s", name, message)
}

func TestLanguage(t *testing.T) {
	t.Parallel()

	engineAddress, engine := runTestingHost(t)

	tests, err := engine.GetLanguageTests(t.Context(), &testingrpc.GetLanguageTestsRequest{})
	require.NoError(t, err)

	cancel := make(chan bool)

	handle, err := rpcutil.ServeWithOptions(rpcutil.ServeOptions{
		Init: func(srv *grpc.Server) error {
			host, err := server.NewLanguageHost(engineAddress)
			if err != nil {
				return err
			}
			t.Cleanup(func() { contract.IgnoreClose(host) })
			pulumirpc.RegisterLanguageRuntimeServer(srv, host)
			pulumirpc.RegisterConverterServer(srv, plugin.NewConverterServer(converter.New()))
			return nil
		},
		Cancel: cancel,
	})
	require.NoError(t, err)

	rootDir := t.TempDir()

	snapshotDir := "./testdata/"

	prepare, err := engine.PrepareLanguageTests(t.Context(), &testingrpc.PrepareLanguageTestsRequest{
		LanguagePluginName:    "hcl",
		LanguagePluginTarget:  fmt.Sprintf("127.0.0.1:%d", handle.Port),
		ConverterPluginTarget: fmt.Sprintf("127.0.0.1:%d", handle.Port),
		TemporaryDirectory:    rootDir,
		SnapshotDirectory:     snapshotDir,
	})
	require.NoError(t, err)

	for _, tt := range tests.Tests {
		t.Run(tt, func(t *testing.T) {
			t.Parallel()
			if strings.HasPrefix(tt, "policy-") {
				t.Skip("HCL does not support policy tests")
			}
			if strings.HasPrefix(tt, "provider-") {
				t.Skip("HCL does not support provider tests")
			}

			if expected, ok := expectedFailures[tt]; ok {
				t.Skipf("Skipping known failure: %s", expected)
			}

			result, err := engine.RunLanguageTest(t.Context(), &testingrpc.RunLanguageTestRequest{
				Token:            prepare.Token,
				Test:             tt,
				SkipConvertTests: has(expectedEjectFailures, tt),
			})

			require.NoError(t, err)
			for _, msg := range result.Messages {
				t.Log(msg)
			}
			log(t, "stdout", result.Stdout)
			log(t, "stderr", result.Stderr)
			assert.True(t, result.Success)
		})
	}

	t.Cleanup(func() {
		close(cancel)
		assert.NoError(t, <-handle.Done)
	})
}
