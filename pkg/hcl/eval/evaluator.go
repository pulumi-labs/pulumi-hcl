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
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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

// SensitiveMark is the cty value mark applied to sensitive values.
const SensitiveMark = "sensitive"

// SyntheticMark is the cty value mark applied to attributes that pulumi-hcl
// injects onto a resource-reference object but that have no OpenTofu
// equivalent.
//
// In general, they are not user visable.
const SyntheticMark = "pulumi-synthetic"

// DepMark records the URN of a Pulumi resource a value transitively depends
// on. Marks ride along with values through cty expression evaluation, so the
// per-property dep set is just the union of DepMarks on the converted value
// — no parallel static analysis of expressions is needed.
type DepMark urn.URN

// stripSyntheticAttributes recursively removes object attributes whose value
// carries SyntheticMark.
func stripSyntheticAttributes(val cty.Value) cty.Value {
	out, err := cty.Transform(val, func(_ cty.Path, val cty.Value) (cty.Value, error) {
		if !val.IsKnown() || val.IsNull() || !val.Type().IsObjectType() {
			return val, nil
		}
		val, marks := val.Unmark()
		attrs := make(map[string]cty.Value, val.LengthInt())
		for k := range val.Type().AttributeTypes() {
			v := val.GetAttr(k)
			if v.HasMark(SyntheticMark) {
				continue
			}
			attrs[k] = v
		}
		return cty.ObjectVal(attrs).WithMarks(marks), nil
	})
	contract.AssertNoErrorf(err, "an error was never returned from cty.Transform")
	return out
}

// CollectDepURNs returns every distinct URN carried by a DepMark anywhere
// in val's value tree.
func CollectDepURNs(val cty.Value) []string {
	var urns []string
	seen := make(map[string]bool)
	_, marks := val.UnmarkDeep()
	for m := range marks {
		dm, ok := m.(DepMark)
		if !ok {
			continue
		}
		s := string(dm)
		if !seen[s] {
			seen[s] = true
			urns = append(urns, s)
		}
	}
	return urns
}

// SensitiveArgumentDiagnostic returns the diagnostic Terraform emits when
// a sensitive value is supplied as a meta-argument such as `count` or
// `for_each`. argName is the meta-argument's name (e.g. "count" or
// "for_each"). The shape of the diagnostic intentionally mirrors
// Terraform's so users coming from Terraform see a familiar message.
func SensitiveArgumentDiagnostic(argName string, expr hcl.Expression) *hcl.Diagnostic {
	return &hcl.Diagnostic{
		Severity: hcl.DiagError,
		Summary:  fmt.Sprintf("Invalid %s argument", argName),
		Detail: fmt.Sprintf(
			"Sensitive values, or values derived from sensitive values, "+
				"cannot be used as %s arguments. If used, the sensitive value "+
				"could be exposed as a resource instance key. If you want to use "+
				"the value anyway, wrap it in nonsensitive(...).",
			argName),
		Subject: expr.Range().Ptr(),
	}
}

// EvaluateCount evaluates a count expression and returns (count, isBool, diags).
// isBool is true when the expression evaluated to a boolean (true→1, false→0),
// which suppresses the numeric index suffix on resource names.
// Returns (1, false, nil) if expr is nil (no count specified).
func (e *Evaluator) EvaluateCount(expr hcl.Expression) (int, bool, hcl.Diagnostics) {
	if expr == nil {
		return 1, false, nil
	}

	val, diags := e.Evaluate(expr)
	if diags.HasErrors() {
		return 0, false, diags
	}

	if val.HasMark(SensitiveMark) {
		return 0, false, hcl.Diagnostics{SensitiveArgumentDiagnostic("count", expr)}
	}

	// Count never flows into deps, so drop marks; AsBigFloat / True panic on them.
	val, _ = val.Unmark()

	if val.Type() == cty.Bool {
		if val.True() {
			return 1, true, nil
		}
		return 0, true, nil
	}

	converted, err := convert.Convert(val, cty.Number)
	if err != nil {
		return 0, false, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid count value",
			Detail:   fmt.Sprintf("Count must be a number or boolean, got %s.", val.Type().FriendlyName()),
			Subject:  expr.Range().Ptr(),
		}}
	}
	bf := converted.AsBigFloat()
	i64, _ := bf.Int64()
	count := int(i64)

	if count < 0 {
		return 0, false, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "Invalid count value",
			Detail:   "Count must be a non-negative integer.",
			Subject:  expr.Range().Ptr(),
		}}
	}

	return count, false, nil
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

	if val.HasMark(SensitiveMark) {
		return nil, hcl.Diagnostics{SensitiveArgumentDiagnostic("for_each", expr)}
	}

	// Unmark only the container — per-element DepMarks must survive on the
	// values stored in result so each.value carries deps into the body.
	val, _ = val.Unmark()

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
			k, _ = k.Unmark()
			result[k.AsString()] = v
		}
	case ty.IsSetType():
		// An empty set is always a no-op, even if its element type is
		// `dynamic`.
		if val.LengthInt() > 0 && ty.ElementType() != cty.String {
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
			unmarkedV, _ := v.Unmark()
			result[unmarkedV.AsString()] = v
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

// EvaluateExpression evaluates an HCL expression.
func (e *Evaluator) EvaluateExpression(expr hcl.Expression) (cty.Value, hcl.Diagnostics) {
	return expr.Value(e.ctx.HCLContext())
}

// UnknownValue creates an unknown value of the given type.
// This is used during planning when values are not yet known.
func UnknownValue(ty cty.Type) cty.Value {
	return cty.UnknownVal(ty)
}
