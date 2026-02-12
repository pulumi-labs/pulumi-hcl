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
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
	ctx.SetVariable("instance_count", cty.NumberIntVal(3))

	eval := NewEvaluator(ctx)

	tests := []struct {
		name      string
		expr      string
		expected  int
		expectErr bool
	}{
		{"literal", `3`, 3, false},
		{"variable", `var.instance_count`, 3, false},
		{"zero", `0`, 0, false},
		{"negative", `-1`, 0, true},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			expr := parseExpr(t, tt.expr)
			result, diags := eval.EvaluateCount(expr)
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
			}
		})
	}
}

func TestEvaluateCountNil(t *testing.T) {
	ctx := NewContext("/tmp", "/tmp")
	eval := NewEvaluator(ctx)

	result, diags := eval.EvaluateCount(nil)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != 1 {
		t.Errorf("Expected 1 for nil count, got %d", result)
	}
}

func TestEvaluateForEach(t *testing.T) {
	ctx := NewContext("/tmp", "/tmp")
	eval := NewEvaluator(ctx)

	t.Run("map", func(t *testing.T) {
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
		result, diags := eval.EvaluateForEach(nil)
		if diags.HasErrors() {
			t.Errorf("Unexpected error: %s", diags.Error())
		}
		if result != nil {
			t.Errorf("Expected nil, got %v", result)
		}
	})

	t.Run("list rejected", func(t *testing.T) {
		expr := parseExpr(t, `["a", "b"]`)
		_, diags := eval.EvaluateForEach(expr)
		if !diags.HasErrors() {
			t.Error("Expected error for list type")
		}
	})
}

func TestContextVariables(t *testing.T) {
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/tmp", "/tmp")
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
	ctx := NewContext("/project/module", "/project/module")

	eval := NewEvaluator(ctx)

	expr := parseExpr(t, `path.module`)
	result, diags := eval.EvaluateString(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != "/project/module" {
		t.Errorf("Expected '/project/module', got %q", result)
	}
}

func TestContextTerraform(t *testing.T) {
	ctx := NewContext("/tmp", "/tmp")
	ctx.SetWorkspace("production")

	eval := NewEvaluator(ctx)

	expr := parseExpr(t, `terraform.workspace`)
	result, diags := eval.EvaluateString(expr)
	if diags.HasErrors() {
		t.Errorf("Unexpected error: %s", diags.Error())
	}
	if result != "production" {
		t.Errorf("Expected 'production', got %q", result)
	}
}

func TestContextClone(t *testing.T) {
	ctx := NewContext("/tmp", "/tmp")
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
			result := IsKnown(tt.value)
			if result != tt.expected {
				t.Errorf("Expected %v, got %v", tt.expected, result)
			}
		})
	}
}
