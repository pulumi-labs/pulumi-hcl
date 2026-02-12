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

package codegen

import (
	"fmt"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/zclconf/go-cty/cty"
)

// GenerateProgram generates HCL source code from a bound PCL program.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	genRequiredProviders(body, program)

	for _, node := range program.Nodes {
		switch n := node.(type) {
		case *pcl.Resource:
			d := genResource(body, n)
			diags = append(diags, d...)
		case *pcl.OutputVariable:
			d := genOutput(body, n)
			diags = append(diags, d...)
		default:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "unsupported PCL node type",
				Detail:   fmt.Sprintf("node type %T is not yet supported", node),
			})
		}
	}

	return map[string][]byte{"main.hcl": f.Bytes()}, diags, nil
}

func genRequiredProviders(body *hclwrite.Body, program *pcl.Program) {
	pkgRefs := program.PackageReferences()
	if len(pkgRefs) == 0 {
		return
	}

	var hasProviders bool
	terraform := body.AppendNewBlock("terraform", nil)
	reqProviders := terraform.Body().AppendNewBlock("required_providers", nil)
	for _, ref := range pkgRefs {
		// The "pulumi" package is built-in and should not be listed in required_providers.
		if ref.Name() == "pulumi" {
			continue
		}
		attrs := map[string]cty.Value{
			"source": cty.StringVal("pulumi/" + ref.Name()),
		}
		if v := ref.Version(); v != nil {
			attrs["version"] = cty.StringVal(v.String())
		}
		reqProviders.Body().SetAttributeValue(ref.Name(), cty.ObjectVal(attrs))
		hasProviders = true
	}

	if !hasProviders {
		body.RemoveBlock(terraform)
		return
	}
	body.AppendNewline()
}

func genResource(body *hclwrite.Body, r *pcl.Resource) hcl.Diagnostics {
	hclType, d := resourceHCLType(r)
	if d.HasErrors() {
		return d
	}
	block := body.AppendNewBlock("resource", []string{hclType, r.LogicalName()})
	var diags hcl.Diagnostics
	for _, attr := range r.Inputs {
		d := genExpression(block.Body(), attr.Name, attr.Value)
		diags = append(diags, d...)
	}
	return diags
}

// resourceHCLType converts a Pulumi resource token (e.g. "simple:index:Resource")
// to an HCL resource type name (e.g. "simple_resource") that the runtime's
// ResolveResource function can map back to the original token.
func resourceHCLType(r *pcl.Resource) (string, hcl.Diagnostics) {
	pkg, mod, name, diags := r.DecomposeToken()
	if diags.HasErrors() {
		return "", diags
	}
	// Strip the "index" module (standard Pulumi convention for the default module).
	// Also strip the module when it equals the package name (e.g. pulumi:pulumi:StackReference).
	if mod == "index" || mod == pkg {
		mod = ""
	}
	return strings.ToLower(pkg + "_" + mod + name), nil
}

func genOutput(body *hclwrite.Body, ov *pcl.OutputVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("output", []string{ov.LogicalName()})
	return genExpression(block.Body(), "value", ov.Value)
}

func genExpression(body *hclwrite.Body, name string, expr model.Expression) hcl.Diagnostics {
	tokens, diags := exprTokens(expr)
	if diags.HasErrors() {
		return diags
	}
	body.SetAttributeRaw(name, tokens)
	return diags
}

// exprTokens converts a PCL expression into HCL tokens.
func exprTokens(expr model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	switch e := expr.(type) {
	case *model.LiteralValueExpression:
		return hclwrite.TokensForValue(e.Value), nil
	case *model.TemplateExpression:
		if len(e.Parts) == 1 {
			return exprTokens(e.Parts[0])
		}
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	case *model.FunctionCallExpression:
		return funcCallTokens(e)
	case *model.ScopeTraversalExpression:
		return scopeTraversalTokens(e)
	case *model.TupleConsExpression:
		return tupleTokens(e)
	case *model.ObjectConsExpression:
		return objectTokens(e)
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	}
}

func funcCallTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	switch expr.Name {
	case "cwd":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "path"},
			hcl.TraverseAttr{Name: "cwd"},
		}), nil
	case "rootDirectory":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "path"},
			hcl.TraverseAttr{Name: "root"},
		}), nil
	case "stack":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "terraform"},
			hcl.TraverseAttr{Name: "workspace"},
		}), nil
	case "getOutput":
		return getOutputTokens(expr)
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported function",
			Detail:   fmt.Sprintf("function %q is not yet supported", expr.Name),
		}}
	}
}

// getOutputTokens generates tokens for getOutput(resource, "outputName").
// This produces resource_type.name.outputs["outputName"].
func getOutputTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	if len(expr.Args) != 2 {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid getOutput call",
			Detail:   "getOutput requires exactly 2 arguments: resource reference and output name",
		}}
	}

	// First arg is a scope traversal to the resource
	resRef, ok := expr.Args[0].(*model.ScopeTraversalExpression)
	if !ok {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid getOutput call",
			Detail:   "first argument must be a resource reference",
		}}
	}

	// The resource reference should resolve to a pcl.Resource
	res, ok := resRef.Parts[0].(*pcl.Resource)
	if !ok {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid getOutput call",
			Detail:   "first argument must reference a resource",
		}}
	}

	// Second arg is the output name (string literal, possibly wrapped in a template).
	outputName, ok := extractStringLiteral(expr.Args[1])
	if !ok {
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid getOutput call",
			Detail:   "second argument must be a string literal",
		}}
	}

	// Get the HCL resource type
	hclType, diags := resourceHCLType(res)
	if diags.HasErrors() {
		return nil, diags
	}

	// Generate: resource_type.name.outputs["outputName"]
	return hclwrite.TokensForTraversal(hcl.Traversal{
		hcl.TraverseRoot{Name: hclType},
		hcl.TraverseAttr{Name: res.LogicalName()},
		hcl.TraverseAttr{Name: "outputs"},
		hcl.TraverseIndex{Key: cty.StringVal(outputName)},
	}), nil
}

// scopeTraversalTokens generates HCL tokens for a scope traversal expression.
func scopeTraversalTokens(expr *model.ScopeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	return hclwrite.TokensForTraversal(expr.Traversal), nil
}

// extractStringLiteral extracts a string from a literal expression,
// unwrapping TemplateExpressions that contain a single literal part.
func extractStringLiteral(expr model.Expression) (string, bool) {
	switch e := expr.(type) {
	case *model.LiteralValueExpression:
		if e.Value.Type() == cty.String {
			return e.Value.AsString(), true
		}
	case *model.TemplateExpression:
		if len(e.Parts) == 1 {
			return extractStringLiteral(e.Parts[0])
		}
	}
	return "", false
}

func tupleTokens(expr *model.TupleConsExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
	}
	var diags hcl.Diagnostics
	for i, elem := range expr.Expressions {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		elemTokens, d := exprTokens(elem)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		if len(elemTokens) > 0 {
			elemTokens[0].SpacesBefore = 1
		}
		tokens = append(tokens, elemTokens...)
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	return tokens, diags
}

func objectTokens(expr *model.ObjectConsExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")},
	}
	if len(expr.Items) == 0 {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})
		return tokens, nil
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	var diags hcl.Diagnostics
	for _, item := range expr.Items {
		keyTokens, d := exprTokens(item.Key)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		valTokens, d := exprTokens(item.Value)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		if len(keyTokens) > 0 {
			keyTokens[0].SpacesBefore = 2
		}
		tokens = append(tokens, keyTokens...)
		tokens = append(tokens, &hclwrite.Token{
			Type: hclsyntax.TokenEqual, Bytes: []byte("="), SpacesBefore: 1,
		})
		if len(valTokens) > 0 {
			valTokens[0].SpacesBefore = 1
		}
		tokens = append(tokens, valTokens...)
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})
	return tokens, diags
}

