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

package ast

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/json"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func parseTestExpr(t *testing.T, src string) hcl.Expression {
	t.Helper()
	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())
	return expr
}

func TestParseProviderFunctionName(t *testing.T) {
	t.Parallel()

	type parsed struct {
		Provider, Func string
		OK             bool
	}
	parse := func(name string) parsed {
		p, f, ok := ParseProviderFunctionName(name)
		return parsed{p, f, ok}
	}

	assert.Equal(t, parsed{"aws", "arn_parse", true}, parse("provider::aws::arn_parse"))
	assert.Equal(t, parsed{OK: false}, parse("arn_parse"))
	assert.Equal(t, parsed{OK: false}, parse("provider::aws"))
	assert.Equal(t, parsed{OK: false}, parse("provider::::arn_parse"))
	assert.Equal(t, parsed{OK: false}, parse("provider::aws::"))
	assert.Equal(t, parsed{OK: false}, parse("provider::aws::a::b"))
	assert.Equal(t, parsed{OK: false}, parse("core::abs"))
}

func TestProviderFunctionNameRoundTrips(t *testing.T) {
	t.Parallel()

	name := ProviderFunctionName("aws", "arn_parse")
	assert.Equal(t, "provider::aws::arn_parse", name)
	p, f, ok := ParseProviderFunctionName(name)
	assert.Equal(t, []any{"aws", "arn_parse", true}, []any{p, f, ok})
}

func TestProviderFunctionCallsInExpr(t *testing.T) {
	t.Parallel()

	assert.Equal(t, []string{"aws"},
		ProviderFunctionCallsInExpr(parseTestExpr(t, `provider::aws::arn_parse(var.x)`)))

	// Nested inside other expressions and templates, deduplicated and sorted.
	assert.Equal(t, []string{"aws", "gcp"},
		ProviderFunctionCallsInExpr(parseTestExpr(t,
			`"${provider::gcp::f(1)}-${upper(provider::aws::g(provider::aws::h()))}"`)))

	assert.Empty(t, ProviderFunctionCallsInExpr(parseTestExpr(t, `upper(var.x)`)))
}

func TestProviderFunctionCallsInBody(t *testing.T) {
	t.Parallel()

	src := []byte(`
locals {
  a = provider::aws::arn_parse("x")
}

resource "null_resource" "r" {
  dynamic "blk" {
    for_each = provider::rand::seq(3)
    content {
      v = blk.value
    }
  }
}
`)
	f, diags := hclsyntax.ParseConfig(src, "test.tf", hcl.InitialPos)
	require.False(t, diags.HasErrors(), diags.Error())

	calls, ok := ProviderFunctionCallsInBody(f.Body)
	assert.True(t, ok)
	assert.Equal(t, []string{"aws", "rand"}, calls)
}

func TestProviderFunctionCallsInBodyJSONIsUnscannable(t *testing.T) {
	t.Parallel()

	f, diags := json.Parse([]byte(`{"locals": {"a": 1}}`), "test.tf.json")
	require.False(t, diags.HasErrors(), diags.Error())

	calls, ok := ProviderFunctionCallsInBody(f.Body)
	assert.False(t, ok)
	assert.Empty(t, calls)
}
