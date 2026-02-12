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

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
)

// GenerateProgram generates HCL source code from a bound PCL program.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	for _, node := range program.Nodes {
		switch n := node.(type) {
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
	case "stack":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "terraform"},
			hcl.TraverseAttr{Name: "workspace"},
		}), nil
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported function",
			Detail:   fmt.Sprintf("function %q is not yet supported", expr.Name),
		}}
	}
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

