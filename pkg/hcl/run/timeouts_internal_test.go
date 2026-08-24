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

package run

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/ast"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/bridge"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// parseTimeouts returns the ast.Timeouts of a resource whose timeouts block
// holds the given body.
func parseTimeouts(t *testing.T, body string) *ast.Timeouts {
	t.Helper()
	src := "resource \"test_thing\" \"x\" {\n  timeouts {\n" + body + "\n  }\n}\n"
	config, diags := parser.NewParser().ParseSource("test.hcl", []byte(src))
	require.False(t, diags.HasErrors(), diags.Error())
	return config.Resources["test_thing.x"].Timeouts
}

func wireMapping(ops ...string) *bridge.BodyMapping {
	nested := &bridge.BodyMapping{Fields: map[string]*bridge.FieldMapping{}}
	for _, op := range ops {
		nested.Fields[op] = &bridge.FieldMapping{TFName: op, PulumiName: op}
	}
	return &bridge.BodyMapping{Fields: map[string]*bridge.FieldMapping{
		"timeouts": {TFName: "timeouts", PulumiName: "timeouts", Nested: nested},
	}}
}

func TestTimeoutsDeclaredOps(t *testing.T) {
	t.Parallel()

	ops, role := timeoutsDeclaredOps(nil)
	assert.Equal(t, allTimeoutOps, ops)
	assert.Equal(t, timeoutsNone, role)

	ops, role = timeoutsDeclaredOps(wireMapping("delete", "create"))
	assert.Equal(t, []string{"create", "delete"}, ops)
	assert.Equal(t, timeoutsInput, role)

	ops, role = timeoutsDeclaredOps(&bridge.BodyMapping{Fields: map[string]*bridge.FieldMapping{
		"timeouts": {TFName: "timeouts", PulumiName: "timeouts"},
	}})
	assert.Nil(t, ops)
	assert.Equal(t, timeoutsNone, role)

	ops, role = timeoutsDeclaredOps(&bridge.BodyMapping{Timeouts: []string{"create"}})
	assert.Equal(t, []string{"create"}, ops)
	assert.Equal(t, timeoutsAttribute, role)

	ops, role = timeoutsDeclaredOps(&bridge.BodyMapping{})
	assert.Nil(t, ops)
	assert.Equal(t, timeoutsNone, role)
}

func TestEvalTimeouts(t *testing.T) {
	t.Parallel()
	hclCtx := &hcl.EvalContext{}

	t.Run("configured operations evaluate, unset declared ones are null", func(t *testing.T) {
		t.Parallel()
		val, role, err := evalTimeouts(parseTimeouts(t, `create = "5m"`),
			&bridge.BodyMapping{Timeouts: []string{"create", "delete"}}, hclCtx)
		require.NoError(t, err)
		assert.Equal(t, timeoutsAttribute, role)
		assert.Equal(t, cty.ObjectVal(map[string]cty.Value{
			"create": cty.StringVal("5m"),
			"delete": cty.NullVal(cty.String),
		}), val)
	})

	t.Run("absent block on a declaring resource is null", func(t *testing.T) {
		t.Parallel()
		val, role, err := evalTimeouts(nil, wireMapping("create"), hclCtx)
		require.NoError(t, err)
		assert.Equal(t, timeoutsInput, role)
		assert.Equal(t, cty.NullVal(cty.Object(map[string]cty.Type{"create": cty.String})), val)
	})

	t.Run("absent block on a non-declaring resource is nothing", func(t *testing.T) {
		t.Parallel()
		val, role, err := evalTimeouts(nil, &bridge.BodyMapping{}, hclCtx)
		require.NoError(t, err)
		assert.Equal(t, timeoutsNone, role)
		assert.Equal(t, cty.NilVal, val)
	})

	t.Run("block on a resource that declares none", func(t *testing.T) {
		t.Parallel()
		_, _, err := evalTimeouts(parseTimeouts(t, `create = "5m"`), &bridge.BodyMapping{}, hclCtx)
		assert.ErrorContains(t, err, "Unsupported block type")
	})

	t.Run("undeclared operation", func(t *testing.T) {
		t.Parallel()
		_, _, err := evalTimeouts(parseTimeouts(t, `read = "1m"`),
			&bridge.BodyMapping{Timeouts: []string{"create"}}, hclCtx)
		assert.ErrorContains(t, err, `Unsupported argument; An argument named "read" is not expected here`)
	})

	t.Run("evaluation failure", func(t *testing.T) {
		t.Parallel()
		_, _, err := evalTimeouts(parseTimeouts(t, `create = var.undefined`),
			&bridge.BodyMapping{Timeouts: []string{"create"}}, hclCtx)
		assert.ErrorContains(t, err, "evaluating timeouts.create")
	})

	t.Run("non-string value", func(t *testing.T) {
		t.Parallel()
		_, _, err := evalTimeouts(parseTimeouts(t, `create = ["5m"]`),
			&bridge.BodyMapping{Timeouts: []string{"create"}}, hclCtx)
		assert.ErrorContains(t, err, `Incorrect attribute value type; Inappropriate value for attribute "create"`)
	})
}

func TestCustomTimeoutsFromValue(t *testing.T) {
	t.Parallel()

	ct, err := customTimeoutsFromValue(cty.NilVal)
	require.NoError(t, err)
	assert.Nil(t, ct)

	ct, err = customTimeoutsFromValue(cty.ObjectVal(map[string]cty.Value{
		"create":  cty.StringVal("1h"),
		"delete":  cty.NullVal(cty.String),
		"read":    cty.UnknownVal(cty.String),
		"default": cty.StringVal("10m"),
	}))
	require.NoError(t, err)
	assert.Equal(t, &CustomTimeouts{Create: 3600, Update: 600, Delete: 600}, ct)

	ct, err = customTimeoutsFromValue(cty.ObjectVal(map[string]cty.Value{
		"create": cty.NullVal(cty.String),
	}))
	require.NoError(t, err)
	assert.Nil(t, ct)

	_, err = customTimeoutsFromValue(cty.ObjectVal(map[string]cty.Value{
		"create": cty.StringVal("banana"),
	}))
	assert.ErrorContains(t, err, `parsing "create" timeout`)
}
