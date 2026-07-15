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

package runtime

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
)

func parseBody(t *testing.T, src string) hcl.Body {
	t.Helper()
	f, diags := hclsyntax.ParseConfig([]byte(src), "test.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	return f.Body
}

func TestEvalStringUnmarksDepMarkedValue(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `command = "echo ${upstream.result}"`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"upstream": cty.ObjectVal(map[string]cty.Value{
				"result": cty.StringVal("from-upstream"),
			}).Mark(eval.DepMark("urn:pulumi:dev::p::simple:index:resource::upstream")),
		},
	}

	command, err := evalString(content, "command", hclCtx)
	require.NoError(t, err)
	assert.Equal(t, "echo from-upstream", command)
}

func TestEvalStringMapCoercesValues(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `
command = "true"
environment = {
  COUNT = 5
  FLAG  = true
  NAME  = "x"
  SKIP  = null
}
`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	env, err := evalStringMap(content, "environment", &hcl.EvalContext{})
	require.NoError(t, err)
	assert.Equal(t, map[string]string{
		"COUNT": "5",
		"FLAG":  "true",
		"NAME":  "x",
	}, env)
}

func TestEvalStringMapRejectsNonMap(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `
command     = "true"
environment = "not-a-map"
`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	_, err := evalStringMap(content, "environment", &hcl.EvalContext{})
	assert.EqualError(t, err, "environment: map of string required, but have string")
}

func TestEvalHelpersCoerce(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `
command     = 5
quiet       = "true"
interpreter = ["/bin/sh", 42, null]
`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	command, err := evalString(content, "command", &hcl.EvalContext{})
	require.NoError(t, err)
	assert.Equal(t, "5", command)

	quiet, err := evalOptionalBool(content, "quiet", &hcl.EvalContext{})
	require.NoError(t, err)
	assert.Equal(t, true, quiet)

	interpreter, err := evalStringSlice(content, "interpreter", &hcl.EvalContext{})
	require.NoError(t, err)
	assert.Equal(t, []string{"/bin/sh", "42"}, interpreter)
}

func TestConfigSensitive(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `command = "echo ${var_secret}"`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var_secret": cty.StringVal("hunter2").Mark(eval.SensitiveMark),
		},
	}
	assert.Equal(t, true, configSensitive(content, hclCtx))

	hclCtx.Variables["var_secret"] = cty.StringVal("hunter2").
		Mark(eval.DepMark("urn:pulumi:dev::p::simple:index:resource::upstream"))
	assert.Equal(t, false, configSensitive(content, hclCtx))
}

func TestRunLocalExecInterpolatesMarkedReference(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "out.txt")
	body := parseBody(t, fmt.Sprintf(`command = "printf %%s ${upstream.result} > '%s'"`, dest))

	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"upstream": cty.ObjectVal(map[string]cty.Value{
				"result": cty.StringVal("from-upstream"),
			}).Mark(eval.DepMark("urn:pulumi:dev::p::simple:index:resource::upstream")),
		},
	}

	err := Run(t.Context(), &Spec{Type: "local-exec", Config: body}, hclCtx)
	require.NoError(t, err)

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "from-upstream", string(got))
}

func TestRunLocalExecSensitiveValueStillExecutes(t *testing.T) {
	t.Parallel()
	dest := filepath.Join(t.TempDir(), "out.txt")
	body := parseBody(t, fmt.Sprintf(`command = "printf %%s ${secret} > '%s'"`, dest))

	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"secret": cty.StringVal("hunter2").Mark(eval.SensitiveMark),
		},
	}

	err := Run(t.Context(), &Spec{Type: "local-exec", Config: body}, hclCtx)
	require.NoError(t, err)

	got, err := os.ReadFile(dest)
	require.NoError(t, err)
	assert.Equal(t, "hunter2", string(got))
}
