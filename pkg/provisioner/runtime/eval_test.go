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

func TestEvaluatorUnmarksDepMarkedValue(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `command = "echo ${upstream.result}"`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	ev := &evaluator{hclCtx: &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"upstream": cty.ObjectVal(map[string]cty.Value{
				"result": cty.StringVal("from-upstream"),
			}).Mark(eval.DepMark("urn:pulumi:dev::p::simple:index:resource::upstream")),
		},
	}}

	command, err := ev.evalString(content, "command")
	require.NoError(t, err)
	assert.Equal(t, "echo from-upstream", command)
	assert.Equal(t, false, ev.sensitive)
}

func TestEvaluatorTracksSensitiveMark(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `command = "echo ${var_secret}"`)
	content, diags := body.Content(localExecSchema)
	require.False(t, diags.HasErrors(), diags.Error())

	ev := &evaluator{hclCtx: &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"var_secret": cty.StringVal("hunter2").Mark(eval.SensitiveMark),
		},
	}}

	command, err := ev.evalString(content, "command")
	require.NoError(t, err)
	assert.Equal(t, "echo hunter2", command)
	assert.Equal(t, true, ev.sensitive)
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

func TestEvalConnectionUnmarksValues(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `
host     = upstream.ip
type     = "ssh"
user     = "root"
password = secret
`)

	hclCtx := &hcl.EvalContext{
		Variables: map[string]cty.Value{
			"upstream": cty.ObjectVal(map[string]cty.Value{
				"ip": cty.StringVal("10.0.0.2"),
			}).Mark(eval.DepMark("urn:pulumi:dev::p::simple:index:resource::upstream")),
			"secret": cty.StringVal("hunter2").Mark(eval.SensitiveMark),
		},
	}

	connVal, err := evalConnection(body, hclCtx)
	require.NoError(t, err)

	// AsString panics on marked values, so these assertions double as an
	// unmarked-ness check.
	assert.Equal(t, "10.0.0.2", connVal.GetAttr("host").AsString())
	assert.Equal(t, "hunter2", connVal.GetAttr("password").AsString())
}
