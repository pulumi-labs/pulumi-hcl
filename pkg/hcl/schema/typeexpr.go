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

package schema

import (
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/transform"
	"github.com/zclconf/go-cty/cty"
)

// typeExpr evaluates expr against ctx for its type, the way HCL would, except
// that traversals are applied one step at a time so an object indexed out of
// an unknown collection regains the null refinements of its type. Every other
// expression is delegated to HCL.
func typeExpr(expr hcl.Expression, ctx *hcl.EvalContext) (cty.Value, hcl.Diagnostics) {
	switch e := expr.(type) {
	case *hclsyntax.ParenthesesExpr:
		return typeExpr(e.Expression, ctx)

	case *hclsyntax.ScopeTraversalExpr:
		root, diags := e.Traversal[:1].TraverseAbs(ctx)
		if diags.HasErrors() {
			return root, diags
		}
		return traverse(root, e.Traversal[1:])

	case *hclsyntax.RelativeTraversalExpr:
		source, diags := typeExpr(e.Source, ctx)
		if diags.HasErrors() {
			return source, diags
		}
		return traverse(source, e.Traversal)

	case *hclsyntax.IndexExpr:
		collection, diags := typeExpr(e.Collection, ctx)
		if diags.HasErrors() {
			return collection, diags
		}
		key, keyDiags := e.Key.Value(ctx)
		if keyDiags.HasErrors() {
			return key, keyDiags
		}
		return traverse(collection, hcl.Traversal{hcl.TraverseIndex{Key: key, SrcRange: e.SrcRange}})
	}

	return expr.Value(ctx)
}

// traverse applies traversal to val one step at a time. An unknown collection,
// such as a counted or for_each reference, yields an unrefined unknown when
// indexed; when that element is an object it is rebuilt with the refinements
// its type implies, as a direct reference to the same object carries, so
// attributes reached through the index keep their nullability.
func traverse(val cty.Value, traversal hcl.Traversal) (cty.Value, hcl.Diagnostics) {
	for _, step := range traversal {
		collection := val
		var diags hcl.Diagnostics
		val, diags = hcl.Traversal{step}.TraverseRel(collection)
		if diags.HasErrors() {
			return val, diags
		}
		if _, isIndex := step.(hcl.TraverseIndex); isIndex && !collection.IsKnown() && val.Type().IsObjectType() {
			val = transform.RefinedUnknown(val.Type(), false).WithSameMarks(val)
		}
	}
	return val, nil
}
