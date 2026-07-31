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
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"

	"github.com/pulumi/pulumi-hcl/pkg/hcl/eval"
)

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

func TestEvalConnectionAcceptsWinRMAttrs(t *testing.T) {
	t.Parallel()
	body := parseBody(t, `
host     = "10.0.0.2"
type     = "ssh"
use_ntlm = true
https    = false
insecure = true
cacert   = "pem"
`)

	connVal, err := evalConnection(body, &hcl.EvalContext{})
	require.NoError(t, err)
	assert.Equal(t, cty.True, connVal.GetAttr("use_ntlm"))
}
