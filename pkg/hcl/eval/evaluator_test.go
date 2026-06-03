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

package eval

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

func parseExpr(t *testing.T, src string) hcl.Expression {
	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("Failed to parse expression %q: %s", src, diags.Error())
	}
	return expr
}

func TestEvaluateString(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("name", cty.StringVal("test"))

	eval := NewEvaluator(ctx)

	tests := []struct {
		name     string
		expr     string
		expected string
	}{
		{"literal", `"hello"`, "hello"},
		{"variable", `var.name`, "test"},
		{"interpolation", `"Hello, ${var.name}!"`, "Hello, test!"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			result, diags := eval.EvaluateString(expr)
			if diags.HasErrors() {
				t.Errorf("Unexpected error: %s", diags.Error())
			}
			if result != tt.expected {
				t.Errorf("Expected %q, got %q", tt.expected, result)
			}
		})
	}
}

func TestEvaluateInt(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("count", cty.NumberIntVal(5))

	eval := NewEvaluator(ctx)

	tests := []struct {
		name     string
		expr     string
		expected int
	}{
		{"literal", `42`, 42},
		{"variable", `var.count`, 5},
		{"arithmetic", `var.count + 10`, 15},
		{"multiply", `var.count * 2`, 10},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			result, diags := eval.EvaluateInt(expr)
			if diags.HasErrors() {
				t.Errorf("Unexpected error: %s", diags.Error())
			}
			if result != tt.expected {
				t.Errorf("Expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestEvaluateBool(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("enabled", cty.BoolVal(true))

	eval := NewEvaluator(ctx)

	tests := []struct {
		name     string
		expr     string
		expected bool
	}{
		{"true literal", `true`, true},
		{"false literal", `false`, false},
		{"variable", `var.enabled`, true},
		{"negation", `!var.enabled`, false},
		{"comparison", `1 < 2`, true},
		{"equality", `1 == 1`, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			result, diags := eval.EvaluateBool(expr)
			if diags.HasErrors() {
				t.Errorf("Unexpected error: %s", diags.Error())
			}
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestEvaluateCount(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("instance_count", cty.NumberIntVal(3))

	eval := NewEvaluator(ctx)

	tests := []struct {
		name      string
		expr      string
		expected  int
		isBool    bool
		expectErr bool
	}{
		{"literal", `3`, 3, false, false},
		{"variable", `var.instance_count`, 3, false, false},
		{"zero", `0`, 0, false, false},
		{"negative", `-1`, 0, false, true},
		{"bool_true", `true`, 1, true, false},
		{"bool_false", `false`, 0, true, false},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			result, isBool, diags := eval.EvaluateCount(expr)
			if tt.expectErr {
				if !diags.HasErrors() {
					t.Error("Expected error, got none")
				}
			} else {
				if diags.HasErrors() {
					t.Errorf("Unexpected error: %s", diags.Error())
				}
				if result != tt.expected {
					t.Errorf("Expected %d, got %d", tt.expected, result)
				}
				if isBool != tt.isBool {
					t.Errorf("Expected isBool=%v, got %v", tt.isBool, isBool)
				}
			}
		})
	}
}

// AsBigFloat panics on marked values; a non-sensitive mark (e.g. DepMark)
// must be stripped first.
func TestEvaluateCount_MarkedKnownValue(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("n", cty.NumberIntVal(3).WithMarks(
		cty.NewValueMarks(DepMark("urn:pulumi:dev::p::aws:ec2/vpc:Vpc::test")),
	))
	eval := NewEvaluator(ctx)

	result, isBool, diags := eval.EvaluateCount(parseExpr(t, `var.n`))
	require.False(t, diags.HasErrors(), "unexpected diags: %v", diags)
	assert.Equal(t, 3, result)
	assert.False(t, isBool)
}

// ElementIterator panics on marked containers; a non-sensitive mark must
// be stripped first.
func TestEvaluateForEach_MarkedContainer(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("m", cty.MapVal(map[string]cty.Value{
		"primary": cty.StringVal("p"),
	}).WithMarks(cty.NewValueMarks(DepMark("urn:pulumi:dev::p::aws:ec2/vpc:Vpc::test"))))
	eval := NewEvaluator(ctx)

	result, diags := eval.EvaluateForEach(parseExpr(t, `var.m`))
	require.False(t, diags.HasErrors(), "unexpected diags: %v", diags)
	assert.Equal(t, map[string]cty.Value{
		"primary": cty.StringVal("p"),
	}, result)
}

func TestEvaluateCountNil(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	eval := NewEvaluator(ctx)

	result, isBool, diags := eval.EvaluateCount(nil)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != 1 {
		t.Errorf("Expected 1 for nil count, got %d", result)
	}
	if isBool {
		t.Error("Expected isBool=false for nil count")
	}
}

func TestEvaluateForEach(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	eval := NewEvaluator(ctx)

	t.Run("map", func(t *testing.T) {
		t.Parallel()
		expr := parseExpr(t, `{a = "x", b = "y"}`)
		result, diags := eval.EvaluateForEach(expr)
		if diags.HasErrors() {
			t.Errorf("Unexpected error: %s", diags.Error())
		}
		if len(result) != 2 {
			t.Errorf("Expected 2 elements, got %d", len(result))
		}
		if result["a"].AsString() != "x" {
			t.Errorf("Expected result[a]='x', got %q", result["a"].AsString())
		}
	})

	t.Run("set of strings", func(t *testing.T) {
		t.Parallel()
		expr := parseExpr(t, `toset(["a", "b", "c"])`)
		result, diags := eval.EvaluateForEach(expr)
		if diags.HasErrors() {
			t.Errorf("Unexpected error: %s", diags.Error())
		}
		if len(result) != 3 {
			t.Errorf("Expected 3 elements, got %d", len(result))
		}
	})

	t.Run("nil returns nil", func(t *testing.T) {
		t.Parallel()
		result, diags := eval.EvaluateForEach(nil)
		if diags.HasErrors() {
			t.Errorf("Unexpected error: %s", diags.Error())
		}
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("list rejected", func(t *testing.T) {
		t.Parallel()
		expr := parseExpr(t, `["a", "b"]`)
		_, diags := eval.EvaluateForEach(expr)
		if !diags.HasErrors() {
			t.Error("Expected error for list for_each, got none")
		}
	})
}

func TestContextVariables(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("name", cty.StringVal("test"))
	ctx.SetVariable("count", cty.NumberIntVal(5))

	eval := NewEvaluator(ctx)

	// Test var.name
	expr := parseExpr(t, `var.name`)
	result, diags := eval.EvaluateString(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != "test" {
		t.Errorf("Expected 'test', got %q", result)
	}
}

func TestContextLocals(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetLocal("common_tags", cty.ObjectVal(map[string]cty.Value{
		"Environment": cty.StringVal("dev"),
		"ManagedBy":   cty.StringVal("Pulumi"),
	}))

	eval := NewEvaluator(ctx)

	// Test local.common_tags.Environment
	expr := parseExpr(t, `local.common_tags.Environment`)
	result, diags := eval.EvaluateString(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != "dev" {
		t.Errorf("Expected 'dev', got %q", result)
	}
}

func TestContextCountIndex(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetCount(2)

	eval := NewEvaluator(ctx)

	expr := parseExpr(t, `count.index`)
	result, diags := eval.EvaluateInt(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != 2 {
		t.Errorf("Expected 2, got %d", result)
	}
}

func TestContextEach(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetEach(cty.StringVal("mykey"), cty.StringVal("myvalue"))

	eval := NewEvaluator(ctx)

	// Test each.key
	keyExpr := parseExpr(t, `each.key`)
	keyResult, diags := eval.EvaluateString(keyExpr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if keyResult != "mykey" {
		t.Errorf("Expected 'mykey', got %q", keyResult)
	}

	// Test each.value
	valExpr := parseExpr(t, `each.value`)
	valResult, diags := eval.EvaluateString(valExpr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if valResult != "myvalue" {
		t.Errorf("Expected 'myvalue', got %q", valResult)
	}
}

func TestContextPath(t *testing.T) {
	t.Parallel()
	t.Run("root module yields '.'", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/project/module", "/project/module", "/project/module", "", "", "")
		eval := NewEvaluator(ctx)
		result, diags := eval.EvaluateString(parseExpr(t, `path.module`))
		assert.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, ".", result)

		result, diags = eval.EvaluateString(parseExpr(t, `path.root`))
		assert.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, ".", result)
	})

	t.Run("nested module yields relative path from root", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/project/modules/sub", "/project", "/project", "", "", "")
		eval := NewEvaluator(ctx)
		result, diags := eval.EvaluateString(parseExpr(t, `path.module`))
		assert.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "modules/sub", result)

		result, diags = eval.EvaluateString(parseExpr(t, `path.root`))
		assert.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, ".", result)
	})
}

func TestContextFileResolvesAgainstRootModuleDir(t *testing.T) {
	t.Parallel()

	rootDir := t.TempDir()
	modDir := filepath.Join(rootDir, "mod")
	require.NoError(t, os.MkdirAll(modDir, 0o755))
	require.NoError(t, os.WriteFile(filepath.Join(modDir, "aux.txt"), []byte("hello\n"), 0o644))

	ctx := NewContext(modDir, rootDir, rootDir, "", "", "")
	eval := NewEvaluator(ctx)

	modulePath, diags := eval.EvaluateString(parseExpr(t, `path.module`))
	require.False(t, diags.HasErrors(), diags.Error())
	require.Equal(t, "mod", modulePath)

	hash, diags := eval.EvaluateString(parseExpr(t, `filesha256("${path.module}/aux.txt")`))
	require.False(t, diags.HasErrors(), diags.Error())
	assert.Equal(t, "5891b5b522d5df086d0ff0b110fbd9d21bb4fc7163af34d08286a2e846f6be03", hash)
}

func TestContextTerraform(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "production", "", "")

	eval := NewEvaluator(ctx)

	expr := parseExpr(t, `pulumi.stack`)
	result, diags := eval.EvaluateString(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != "production" {
		t.Errorf("Expected 'production', got %q", result)
	}
}

func TestContextRangedResources(t *testing.T) {
	t.Parallel()
	t.Run("count resources are accessible by index", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
		ctx.SetCountResource("aws_instance.web", 0, cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-000"),
		}))
		ctx.SetCountResource("aws_instance.web", 1, cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-001"),
		}))

		eval := NewEvaluator(ctx)
		result, diags := eval.EvaluateString(parseExpr(t, `aws_instance.web[0].id`))
		require.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "i-000", result)

		result, diags = eval.EvaluateString(parseExpr(t, `aws_instance.web[1].id`))
		require.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "i-001", result)
	})

	t.Run("for_each resources are accessible by key", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
		ctx.SetEachResource("aws_instance.web", "east", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-east"),
		}))
		ctx.SetEachResource("aws_instance.web", "west", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-west"),
		}))

		eval := NewEvaluator(ctx)
		result, diags := eval.EvaluateString(parseExpr(t, `aws_instance.web["east"].id`))
		require.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "i-east", result)
	})

	t.Run("resource named with brackets is not confused with ranged", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
		ctx.SetResource("aws_instance.foo[0]", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-literal"),
		}))

		hclCtx := ctx.HCLContext()
		awsInst := hclCtx.Variables["aws_instance"]
		attr := awsInst.GetAttr("foo[0]")
		assert.Equal(t, "i-literal", attr.GetAttr("id").AsString())
	})

	t.Run("single and ranged resources coexist under same type", func(t *testing.T) {
		t.Parallel()
		ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
		ctx.SetResource("aws_instance.single", cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-single"),
		}))
		ctx.SetCountResource("aws_instance.multi", 0, cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("i-multi-0"),
		}))

		eval := NewEvaluator(ctx)
		result, diags := eval.EvaluateString(parseExpr(t, `aws_instance.single.id`))
		require.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "i-single", result)

		result, diags = eval.EvaluateString(parseExpr(t, `aws_instance.multi[0].id`))
		require.False(t, diags.HasErrors(), diags.Error())
		assert.Equal(t, "i-multi-0", result)
	})
}

func TestContextClone(t *testing.T) {
	t.Parallel()
	ctx := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	ctx.SetVariable("name", cty.StringVal("original"))

	clone := ctx.Clone()
	clone.SetVariable("name", cty.StringVal("cloned"))

	// Original should be unchanged
	origEval := NewEvaluator(ctx)
	expr := parseExpr(t, `var.name`)
	result, _ := origEval.EvaluateString(expr)
	if result != "original" {
		t.Errorf("Original context was modified, expected 'original', got %q", result)
	}

	// Clone should have new value
	cloneEval := NewEvaluator(clone)
	cloneResult, _ := cloneEval.EvaluateString(expr)
	if cloneResult != "cloned" {
		t.Errorf("Clone should have 'cloned', got %q", cloneResult)
	}
}

func TestParseTraversal(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name              string
		expr              string
		expectedNamespace string
		expectedParts     []string
	}{
		{
			name:              "simple variable",
			expr:              `var.name`,
			expectedNamespace: "var",
			expectedParts:     []string{"name"},
		},
		{
			name:              "nested attribute",
			expr:              `local.tags.Environment`,
			expectedNamespace: "local",
			expectedParts:     []string{"tags", "Environment"},
		},
		{
			name:              "resource reference",
			expr:              `aws_instance.web.id`,
			expectedNamespace: "aws_instance",
			expectedParts:     []string{"web", "id"},
		},
		{
			name:              "data source",
			expr:              `data.aws_ami.ubuntu.id`,
			expectedNamespace: "data",
			expectedParts:     []string{"aws_ami", "ubuntu", "id"},
		},
		{
			name:              "module output",
			expr:              `module.vpc.vpc_id`,
			expectedNamespace: "module",
			expectedParts:     []string{"vpc", "vpc_id"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			traversals := expr.Variables()
			if len(traversals) != 1 {
				t.Fatalf("Expected 1 traversal, got %d", len(traversals))
			}

			namespace, parts := ParseTraversal(traversals[0])
			if namespace != tt.expectedNamespace {
				t.Errorf("Expected namespace %q, got %q", tt.expectedNamespace, namespace)
			}
			if len(parts) != len(tt.expectedParts) {
				t.Errorf("Expected %d parts, got %d", len(tt.expectedParts), len(parts))
			} else {
				for i, part := range parts {
					if part != tt.expectedParts[i] {
						t.Errorf("Part %d: expected %q, got %q", i, tt.expectedParts[i], part)
					}
				}
			}
		})
	}
}

func TestExtractDependencies(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected []string
	}{
		{
			name:     "no dependencies",
			expr:     `"literal"`,
			expected: nil,
		},
		{
			name:     "variable reference",
			expr:     `var.name`,
			expected: nil, // var is not a dependency
		},
		{
			name:     "resource reference",
			expr:     `aws_instance.web.id`,
			expected: []string{"aws_instance.web"},
		},
		{
			name:     "data source reference",
			expr:     `data.aws_ami.ubuntu.id`,
			expected: []string{"data.aws_ami.ubuntu"},
		},
		{
			name:     "module reference",
			expr:     `module.vpc.vpc_id`,
			expected: []string{"module.vpc"},
		},
		{
			name:     "multiple references",
			expr:     `"${aws_instance.web.id}-${aws_s3_bucket.mybucket.arn}"`,
			expected: []string{"aws_instance.web", "aws_s3_bucket.mybucket"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			expr := parseExpr(t, tt.expr)
			deps := ExtractDependencies(expr)

			if len(deps) != len(tt.expected) {
				t.Errorf("Expected %d dependencies, got %d: %v", len(tt.expected), len(deps), deps)
			} else {
				for i, dep := range deps {
					if dep != tt.expected[i] {
						t.Errorf("Dependency %d: expected %q, got %q", i, tt.expected[i], dep)
					}
				}
			}
		})
	}
}

func TestIsKnown(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		value    cty.Value
		expected bool
	}{
		{"known string", cty.StringVal("hello"), true},
		{"known number", cty.NumberIntVal(42), true},
		{"unknown string", cty.UnknownVal(cty.String), false},
		{"known list", cty.ListVal([]cty.Value{cty.StringVal("a")}), true},
		{"list with unknown", cty.ListVal([]cty.Value{cty.UnknownVal(cty.String)}), false},
		{"known map", cty.MapVal(map[string]cty.Value{"a": cty.StringVal("b")}), true},
		{"null", cty.NullVal(cty.String), true}, // null is known
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := IsKnown(tt.value)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}

func TestMarkOutputLeaves(t *testing.T) {
	t.Parallel()
	mark := DepMark("urn:test::a")

	t.Run("primitive marks at top", func(t *testing.T) {
		t.Parallel()
		out := MarkOutputLeaves(cty.StringVal("hello"), mark)
		assert.True(t, out.HasMark(mark))
	})

	t.Run("object marks each leaf but not container", func(t *testing.T) {
		t.Parallel()
		obj := cty.ObjectVal(map[string]cty.Value{
			"id":   cty.StringVal("xyz"),
			"name": cty.StringVal("foo"),
		})
		out := MarkOutputLeaves(obj, mark)
		assert.False(t, out.IsMarked(), "container itself should be unmarked")
		assert.True(t, out.GetAttr("id").HasMark(mark))
		assert.True(t, out.GetAttr("name").HasMark(mark))
	})

	t.Run("list marks each element", func(t *testing.T) {
		t.Parallel()
		list := cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
		out := MarkOutputLeaves(list, mark)
		assert.False(t, out.IsMarked())
		for it := out.ElementIterator(); it.Next(); {
			_, v := it.Element()
			assert.True(t, v.HasMark(mark))
		}
	})

	t.Run("nested objects mark every leaf", func(t *testing.T) {
		t.Parallel()
		nested := cty.ObjectVal(map[string]cty.Value{
			"tags": cty.MapVal(map[string]cty.Value{
				"Name": cty.StringVal("hi"),
			}),
		})
		out := MarkOutputLeaves(nested, mark)
		name := out.GetAttr("tags").Index(cty.StringVal("Name"))
		assert.True(t, name.HasMark(mark))
	})

	t.Run("empty containers untouched", func(t *testing.T) {
		t.Parallel()
		empty := cty.MapValEmpty(cty.String)
		out := MarkOutputLeaves(empty, mark)
		assert.False(t, out.IsMarked())
		assert.True(t, out.RawEquals(empty))
	})

	t.Run("null and unknown leaves get marked", func(t *testing.T) {
		t.Parallel()
		assert.True(t, MarkOutputLeaves(cty.NullVal(cty.String), mark).HasMark(mark))
		assert.True(t, MarkOutputLeaves(cty.UnknownVal(cty.String), mark).HasMark(mark))
	})
}

func TestCollectDepURNs(t *testing.T) {
	t.Parallel()
	a := DepMark("urn:test::a")
	b := DepMark("urn:test::b")

	t.Run("no marks", func(t *testing.T) {
		t.Parallel()
		assert.Empty(t, CollectDepURNs(cty.StringVal("hi")))
	})

	t.Run("single leaf mark", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, []string{"urn:test::a"},
			CollectDepURNs(cty.StringVal("hi").Mark(a)))
	})

	t.Run("nested distinct marks deduplicated and ordered first-seen", func(t *testing.T) {
		t.Parallel()
		// {x: marked(a), y: [marked(b), marked(a)]}
		v := cty.ObjectVal(map[string]cty.Value{
			"x": cty.StringVal("v").Mark(a),
			"y": cty.ListVal([]cty.Value{
				cty.StringVal("p").Mark(b),
				cty.StringVal("q").Mark(a),
			}),
		})
		urns := CollectDepURNs(v)
		assert.ElementsMatch(t, []string{"urn:test::a", "urn:test::b"}, urns)
	})

	t.Run("ignores non-DepMark marks like SensitiveMark", func(t *testing.T) {
		t.Parallel()
		v := cty.StringVal("secret").Mark(SensitiveMark)
		assert.Empty(t, CollectDepURNs(v))
	})

	t.Run("propagates through MarkOutputLeaves-marked container", func(t *testing.T) {
		t.Parallel()
		obj := MarkOutputLeaves(cty.ObjectVal(map[string]cty.Value{
			"id": cty.StringVal("xyz"),
		}), a)
		// User-facing read of the marked leaf:
		idAttr := obj.GetAttr("id")
		assert.Equal(t, []string{"urn:test::a"}, CollectDepURNs(idAttr))
	})
}
