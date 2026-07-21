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

package parser

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// shimTraversalInString re-parses a native-syntax quoted string as a traversal,
// supporting the pre-0.12 form of reference arguments (`depends_on =
// ["null_resource.a"]`), and reports a deprecation warning. Any other
// expression is returned verbatim, including JSON-syntax expressions: there,
// traversals in strings are the required form rather than a legacy one.
func shimTraversalInString(expr hcl.Expression) (hcl.Expression, hcl.Diagnostics) {
	if _, ok := expr.(*hclsyntax.TemplateExpr); !ok {
		return expr, nil
	}

	strVal, diags := expr.Value(nil)
	if diags.HasErrors() || strVal.IsNull() || !strVal.IsKnown() {
		// Not shimmable (e.g. the string interpolates something); leave the
		// caller's own error handling to report it.
		return expr, nil
	}

	// The start position ignores escape sequences in the literal, which is
	// close enough for error reporting.
	srcRange := expr.Range()
	startPos := srcRange.Start
	startPos.Column++
	startPos.Byte++

	// On a parse failure the traversal is empty, which callers skip; reporting
	// the parse error here beats letting them report a generic one.
	traversal, tDiags := hclsyntax.ParseTraversalAbs([]byte(strVal.AsString()), srcRange.Filename, startPos)
	diags = append(diags, tDiags...)
	diags = append(diags, &hcl.Diagnostic{
		Severity: hcl.DiagWarning,
		Summary:  "Quoted references are deprecated",
		Detail: "In this context, references are expected literally rather than in quotes. " +
			"Terraform 0.11 and earlier required quotes, but quoted references are now deprecated. " +
			"Remove the quotes surrounding this reference to silence this warning.",
		Subject: &srcRange,
	})

	return &hclsyntax.ScopeTraversalExpr{
		Traversal: traversal,
		SrcRange:  srcRange,
	}, diags
}

// decodeDependsOn decodes a `depends_on` argument into the references it names.
func decodeDependsOn(attr *hcl.Attribute) ([]hcl.Traversal, hcl.Diagnostics) {
	exprs, diags := hcl.ExprList(attr.Expr)

	var ret []hcl.Traversal
	for _, expr := range exprs {
		expr, shimDiags := shimTraversalInString(expr)
		diags = append(diags, shimDiags...)

		traversal, travDiags := hcl.AbsTraversalForExpr(expr)
		diags = append(diags, travDiags...)
		if len(traversal) != 0 {
			ret = append(ret, traversal)
		}
	}
	return ret, diags
}
