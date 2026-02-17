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
	"fmt"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
)

// Evaluator handles expression evaluation for HCL configurations.
type Evaluator struct {
	ctx *Context
}

// NewEvaluator creates a new evaluator with the given context.
func NewEvaluator(ctx *Context) *Evaluator {
	return &Evaluator{ctx: ctx}
}

// Context returns the evaluation context.
func (e *Evaluator) Context() *Context {
	return e.ctx
}

// Evaluate evaluates an HCL expression and returns the result.
func (e *Evaluator) Evaluate(expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
	return expr.Value(e.ctx.HCLContext())
}

// EvaluateAs evaluates an expression and converts the result to the specified type.
func (e *Evaluator) EvaluateAs(expr hcl.Expression, targetType cty.Type) (cty.Value, hcl.Diagnostics) {
	val, diags := e.Evaluate(expr)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	converted, err := convert.Convert(val, targetType)
	if err != nil {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Type conversion error",
			Detail:   fmt.Sprintf("Cannot convert %s to %s: %s", val.Type().FriendlyName(), targetType.FriendlyName(), err),
			Subject:  expr.Range().Ptr(),
		})
		return cty.NilVal, diags
	}

	return converted, diags
}

// EvaluateString evaluates an expression expecting a string result.
func (e *Evaluator) EvaluateString(expr hcl.Expression) (string, hcl.Diagnostics) {
	val, diags := e.EvaluateAs(expr, cty.String)
	if diags.HasErrors() {
		return "", diags
	}
	if val.IsNull() {
		return "", nil
	}
	return val.AsString(), diags
}

// EvaluateInt evaluates an expression expecting an integer result.
func (e *Evaluator) EvaluateInt(expr hcl.Expression) (int, hcl.Diagnostics) {
	val, diags := e.EvaluateAs(expr, cty.Number)
	if diags.HasErrors() {
		return 0, diags
	}
	if val.IsNull() {
		return 0, nil
	}
	bf := val.AsBigFloat()
	i64, _ := bf.Int64()
	return int(i64), diags
}

// EvaluateBool evaluates an expression expecting a boolean result.
func (e *Evaluator) EvaluateBool(expr hcl.Expression) (bool, hcl.Diagnostics) {
	val, diags := e.EvaluateAs(expr, cty.Bool)
	if diags.HasErrors() {
		return false, diags
	}
	if val.IsNull() {
		return false, nil
	}
	return val.True(), diags
}

// EvaluateBody evaluates all attributes in an HCL body.
func (e *Evaluator) EvaluateBody(body hcl.Body) (map[string]cty.Value, hcl.Diagnostics) {
	attrs, diags := body.JustAttributes()
	if diags.HasErrors() {
		return nil, diags
	}

	result := make(map[string]cty.Value)
	for name, attr := range attrs {
		val, valDiags := e.Evaluate(attr.Expr)
		diags = append(diags, valDiags...)
		if !valDiags.HasErrors() {
			result[name] = val
		}
	}

	return result, diags
}

// EvaluateCount evaluates a count expression and returns the count value.
// Returns 1 if expr is nil (no count specified).
func (e *Evaluator) EvaluateCount(expr hcl.Expression) (int, hcl.Diagnostics) {
	if expr == nil {
		return 1, nil
	}

	count, diags := e.EvaluateInt(expr)
	if diags.HasErrors() {
		return 0, diags
	}

	if count < 0 {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid count value",
			Detail:   "Count must be a non-negative integer.",
			Subject:  expr.Range().Ptr(),
		})
		return 0, diags
	}

	return count, diags
}

// EvaluateForEach evaluates a for_each expression and returns the map/set of values.
// Returns nil if expr is nil (no for_each specified).
func (e *Evaluator) EvaluateForEach(expr hcl.Expression) (map[string]cty.Value, hcl.Diagnostics) {
	if expr == nil {
		return nil, nil
	}

	val, diags := e.Evaluate(expr)
	if diags.HasErrors() {
		return nil, diags
	}

	if val.IsNull() {
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid for_each value",
			Detail:   "for_each cannot be null.",
			Subject:  expr.Range().Ptr(),
		})
		return nil, diags
	}

	ty := val.Type()
	result := make(map[string]cty.Value)

	switch {
	case ty.IsMapType() || ty.IsObjectType():
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			result[k.AsString()] = v
		}
	case ty.IsSetType():
		if ty.ElementType() != cty.String {
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "Invalid for_each value",
				Detail:   "for_each set must contain strings.",
				Subject:  expr.Range().Ptr(),
			})
			return nil, diags
		}
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			key := v.AsString()
			result[key] = v
		}
	default:
		diags = append(diags, &hcl.Diagnostic{
			Severity: hcl.DiagError,
			Summary:  "Invalid for_each value",
			Detail:   fmt.Sprintf("for_each must be a map or set, not %s.", ty.FriendlyName()),
			Subject:  expr.Range().Ptr(),
		})
		return nil, diags
	}

	return result, diags
}

// GetReferencedVariables returns all variables referenced by an expression.
func (e *Evaluator) GetReferencedVariables(expr hcl.Expression) []hcl.Traversal {
	return expr.Variables()
}

// ParseTraversal parses a traversal into its components.
func ParseTraversal(traversal hcl.Traversal) (namespace string, parts []string) {
	if len(traversal) == 0 {
		return "", nil
	}

	namespace = traversal.RootName()

	for i := 1; i < len(traversal); i++ {
		switch step := traversal[i].(type) {
		case hcl.TraverseAttr:
			parts = append(parts, step.Name)
		case hcl.TraverseIndex:
			// For index traversals, convert to string representation
			idx := step.Key
			if idx.Type() == cty.String {
				parts = append(parts, idx.AsString())
			} else if idx.Type() == cty.Number {
				bf := idx.AsBigFloat()
				i64, _ := bf.Int64()
				parts = append(parts, fmt.Sprintf("%d", i64))
			}
		}
	}

	return namespace, parts
}

// ResolveReference resolves a reference traversal to its value.
func (e *Evaluator) ResolveReference(traversal hcl.Traversal) (cty.Value, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	if len(traversal) == 0 {
		return cty.NilVal, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Empty reference",
			Detail:   "Reference traversal is empty.",
		}}
	}

	// Use the HCL context to resolve
	ctx := e.ctx.HCLContext()
	rootName := traversal.RootName()

	val, ok := ctx.Variables[rootName]
	if !ok {
		return cty.NilVal, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Unknown reference",
			Detail:   fmt.Sprintf("Reference to unknown value %q.", rootName),
			Subject:  traversal.SourceRange().Ptr(),
		}}
	}

	// Apply remaining traversal steps
	for i := 1; i < len(traversal); i++ {
		step := traversal[i]
		var newVal cty.Value
		var err error

		switch s := step.(type) {
		case hcl.TraverseAttr:
			if val.Type().IsObjectType() || val.Type().IsMapType() {
				if val.Type().IsObjectType() {
					if val.Type().HasAttribute(s.Name) {
						newVal = val.GetAttr(s.Name)
					} else {
						return cty.NilVal, hcl.Diagnostics{{
							Severity: hcl.DiagError,
							Summary:  "Unknown attribute",
							Detail:   fmt.Sprintf("Object has no attribute %q.", s.Name),
							Subject:  &s.SrcRange,
						}}
					}
				} else {
					idx := cty.StringVal(s.Name)
					if val.HasIndex(idx).True() {
						newVal = val.Index(idx)
					} else {
						return cty.NilVal, hcl.Diagnostics{{
							Severity: hcl.DiagError,
							Summary:  "Unknown key",
							Detail:   fmt.Sprintf("Map has no key %q.", s.Name),
							Subject:  &s.SrcRange,
						}}
					}
				}
			} else {
				return cty.NilVal, hcl.Diagnostics{{
					Severity: hcl.DiagError,
					Summary:  "Invalid attribute access",
					Detail:   fmt.Sprintf("Cannot access attribute on %s.", val.Type().FriendlyName()),
					Subject:  &s.SrcRange,
				}}
			}

		case hcl.TraverseIndex:
			idx := s.Key
			if val.HasIndex(idx).True() {
				newVal = val.Index(idx)
			} else {
				return cty.NilVal, hcl.Diagnostics{{
					Severity: hcl.DiagError,
					Summary:  "Invalid index",
					Detail:   fmt.Sprintf("Index %s out of range.", idx.GoString()),
					Subject:  &s.SrcRange,
				}}
			}

		default:
			return cty.NilVal, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Unsupported traversal",
				Detail:   fmt.Sprintf("Unsupported traversal step type: %T", step),
			}}
		}

		if err != nil {
			return cty.NilVal, hcl.Diagnostics{{
				Severity: hcl.DiagError,
				Summary:  "Traversal error",
				Detail:   err.Error(),
			}}
		}

		val = newVal
	}

	return val, diags
}

// ExtractDependencies extracts all resource/data/module dependencies from an expression.
func ExtractDependencies(expr hcl.Expression) []string {
	var deps []string
	seen := make(map[string]bool)

	for _, traversal := range expr.Variables() {
		namespace, parts := ParseTraversal(traversal)

		var dep string
		switch namespace {
		case "var", "local", "path", "terraform", "count", "each", "self":
			// These are not resource dependencies
			continue
		case "data":
			// Data source reference: data.type.name
			if len(parts) >= 2 {
				dep = fmt.Sprintf("data.%s.%s", parts[0], parts[1])
			}
		case "module":
			// Module reference: module.name
			if len(parts) >= 1 {
				dep = fmt.Sprintf("module.%s", parts[0])
			}
		default:
			// Resource reference: type.name (namespace is the type)
			if len(parts) >= 1 {
				dep = fmt.Sprintf("%s.%s", namespace, parts[0])
			}
		}

		if dep != "" && !seen[dep] {
			deps = append(deps, dep)
			seen[dep] = true
		}
	}

	return deps
}

// IsKnown returns true if the value is fully known (not unknown).
func IsKnown(val cty.Value) bool {
	if !val.IsKnown() {
		return false
	}
	if val.Type().IsCollectionType() || val.Type().IsObjectType() || val.Type().IsTupleType() {
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			if !IsKnown(v) {
				return false
			}
		}
	}
	return true
}

// EvaluateExpression evaluates an HCL expression, intercepting any `can()` function calls.
//
// HCL normally evaluates function arguments eagerly: if `can(expr)` is used and `expr`
// produces an error, the error propagates before the `can` function body ever runs.
// This method type-asserts to hclsyntax.FunctionCallExpr, detects `can()`, and evaluates
// its argument directly — returning cty.True on success and cty.False on error.
//
// Sensitivity marks from the inner expression are propagated to the result.
func (e *Evaluator) EvaluateExpression(expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
	ctx := e.ctx.HCLContext()
	return evaluateWithCan(expr, ctx)
}

func evaluateWithCan(expr hcl.Expression, ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	syntaxExpr, ok := expr.(hclsyntax.Expression)
	if !ok {
		return expr.Value(ctx)
	}

	fc, ok := syntaxExpr.(*hclsyntax.FunctionCallExpr)
	if !ok || fc.Name != "can" || len(fc.Args) != 1 {
		return expr.Value(ctx)
	}

	// Evaluate the inner argument; if it errors, return false.
	val, diags := fc.Args[0].Value(ctx)
	if diags.HasErrors() {
		return cty.False, nil
	}

	// Propagate sensitivity: if the inner value is marked as sensitive, the
	// boolean result should also be marked as sensitive.
	result := cty.True
	if val.IsKnown() {
		_, marks := val.Unmark()
		if len(marks) > 0 {
			result = result.WithMarks(marks)
		}
	}

	return result, nil
}

// UnknownValue creates an unknown value of the given type.
// This is used during planning when values are not yet known.
func UnknownValue(ty cty.Type) cty.Value {
	return cty.UnknownVal(ty)
}
