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
		case *pcl.ConfigVariable:
			d := genConfigVariable(body, n)
			diags = append(diags, d...)
		case *pcl.LocalVariable:
			d := genLocalVariable(body, n)
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

func genConfigVariable(body *hclwrite.Body, cv *pcl.ConfigVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("variable", []string{cv.LogicalName()})

	// Set the type constraint if the config has a type label.
	if len(cv.SyntaxNode().(*hclsyntax.Block).Labels) == 2 {
		typeStr := cv.SyntaxNode().(*hclsyntax.Block).Labels[1]
		block.Body().SetAttributeRaw("type", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(typeStr)},
		})
	}

	// Set the default value if present.
	if cv.DefaultValue != nil {
		tokens, diags := exprTokens(cv.DefaultValue)
		if diags.HasErrors() {
			return diags
		}
		block.Body().SetAttributeRaw("default", tokens)
	}

	return nil
}

func genLocalVariable(body *hclwrite.Body, lv *pcl.LocalVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("locals", nil)
	return genExpression(block.Body(), lv.LogicalName(), lv.Definition.Value)
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
		return templateTokens(e)
	case *model.FunctionCallExpression:
		return funcCallTokens(e)
	case *model.ScopeTraversalExpression:
		return scopeTraversalTokens(e)
	case *model.TupleConsExpression:
		return tupleTokens(e)
	case *model.ObjectConsExpression:
		return objectTokens(e)
	case *model.IndexExpression:
		return indexExprTokens(e)
	case *model.RelativeTraversalExpression:
		return relativeTraversalTokens(e)
	case *model.BinaryOpExpression:
		return binaryOpTokens(e)
	case *model.UnaryOpExpression:
		return unaryOpTokens(e)
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
			hcl.TraverseRoot{Name: "pulumi"},
			hcl.TraverseAttr{Name: "stack"},
		}), nil
	case "project":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "pulumi"},
			hcl.TraverseAttr{Name: "project"},
		}), nil
	case "organization":
		return hclwrite.TokensForTraversal(hcl.Traversal{
			hcl.TraverseRoot{Name: "pulumi"},
			hcl.TraverseAttr{Name: "organization"},
		}), nil
	case "getOutput":
		return getOutputTokens(expr)
	case "secret":
		return passthroughFuncCallTokens("sensitive", expr.Args)
	default:
		return passthroughFuncCallTokens(expr.Name, expr.Args)
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
// PCL config variables become HCL `var.<name>`, and local variables become `local.<name>`.
func scopeTraversalTokens(expr *model.ScopeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	traversal := expr.Traversal
	if len(expr.Parts) > 0 {
		var prefix string
		switch expr.Parts[0].(type) {
		case *pcl.ConfigVariable:
			prefix = "var"
		case *pcl.LocalVariable:
			prefix = "local"
		}
		if prefix != "" {
			// Rewrite "aMap.x" → "var.aMap.x" (or "local.aMap.x").
			// The original traversal starts with TraverseRoot{Name: "aMap"}.
			// We replace that with TraverseRoot{Name: "var"}, TraverseAttr{Name: "aMap"}.
			rewritten := make(hcl.Traversal, 0, len(traversal)+1)
			rewritten = append(rewritten, hcl.TraverseRoot{Name: prefix})
			rewritten = append(rewritten, hcl.TraverseAttr{Name: traversal.RootName()})
			rewritten = append(rewritten, traversal[1:]...)
			traversal = rewritten
		}
	}
	return hclwrite.TokensForTraversal(traversal), nil
}

// passthroughFuncCallTokens generates tokens for a function call: name(arg1, arg2, ...).
func passthroughFuncCallTokens(name string, args []model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(name)},
		{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
	}
	var diags hcl.Diagnostics
	for i, arg := range args {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		argTokens, d := exprTokens(arg)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		tokens = append(tokens, argTokens...)
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCParen, Bytes: []byte(")")})
	return tokens, diags
}

// indexExprTokens generates tokens for an index expression: collection[key].
func indexExprTokens(expr *model.IndexExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	collTokens, diags := exprTokens(expr.Collection)
	if diags.HasErrors() {
		return nil, diags
	}
	keyTokens, d := exprTokens(expr.Key)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	tokens := append(collTokens,
		&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
	)
	tokens = append(tokens, keyTokens...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	return tokens, diags
}

// relativeTraversalTokens generates tokens for a relative traversal expression: source.attr.
func relativeTraversalTokens(expr *model.RelativeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	sourceTokens, diags := exprTokens(expr.Source)
	if diags.HasErrors() {
		return nil, diags
	}
	// Append traversal steps as attribute access tokens.
	for _, step := range expr.Traversal {
		switch s := step.(type) {
		case hcl.TraverseAttr:
			sourceTokens = append(sourceTokens,
				&hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
				&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(s.Name)},
			)
		case hcl.TraverseIndex:
			keyTokens := hclwrite.TokensForValue(s.Key)
			sourceTokens = append(sourceTokens,
				&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
			)
			sourceTokens = append(sourceTokens, keyTokens...)
			sourceTokens = append(sourceTokens,
				&hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
			)
		}
	}
	return sourceTokens, diags
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

// templateTokens generates HCL tokens for a template expression.
// For a single literal part, it returns the literal value directly.
// For multiple parts, it generates a template string like "${expr}suffix".
func templateTokens(expr *model.TemplateExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	// If template has a single literal part, just return that literal.
	if len(expr.Parts) == 1 {
		if lit, ok := expr.Parts[0].(*model.LiteralValueExpression); ok {
			return hclwrite.TokensForValue(lit.Value), nil
		}
	}

	// Build an interpolated string: "prefix${expr}suffix"
	var buf strings.Builder
	buf.WriteString(`"`)
	var diags hcl.Diagnostics

	for _, part := range expr.Parts {
		switch p := part.(type) {
		case *model.LiteralValueExpression:
			if p.Value.Type() == cty.String {
				buf.WriteString(p.Value.AsString())
			} else {
				buf.WriteString(fmt.Sprintf("${%s}", p.Value.GoString()))
			}
		default:
			partTokens, d := exprTokens(part)
			diags = append(diags, d...)
			if d.HasErrors() {
				return nil, diags
			}
			buf.WriteString("${")
			for _, tok := range partTokens {
				buf.Write(tok.Bytes)
			}
			buf.WriteString("}")
		}
	}
	buf.WriteString(`"`)

	return hclwrite.Tokens{
		{Type: hclsyntax.TokenQuotedLit, Bytes: []byte(buf.String())},
	}, diags
}

// binaryOpTokens generates HCL tokens for a binary operation expression.
func binaryOpTokens(expr *model.BinaryOpExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	leftTokens, diags := exprTokens(expr.LeftOperand)
	if diags.HasErrors() {
		return nil, diags
	}

	rightTokens, d := exprTokens(expr.RightOperand)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	var opStr string
	switch expr.Operation {
	case hclsyntax.OpLogicalOr:
		opStr = "||"
	case hclsyntax.OpLogicalAnd:
		opStr = "&&"
	case hclsyntax.OpEqual:
		opStr = "=="
	case hclsyntax.OpNotEqual:
		opStr = "!="
	case hclsyntax.OpGreaterThan:
		opStr = ">"
	case hclsyntax.OpGreaterThanOrEqual:
		opStr = ">="
	case hclsyntax.OpLessThan:
		opStr = "<"
	case hclsyntax.OpLessThanOrEqual:
		opStr = "<="
	case hclsyntax.OpAdd:
		opStr = "+"
	case hclsyntax.OpSubtract:
		opStr = "-"
	case hclsyntax.OpMultiply:
		opStr = "*"
	case hclsyntax.OpDivide:
		opStr = "/"
	case hclsyntax.OpModulo:
		opStr = "%"
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported binary operation",
			Detail:   fmt.Sprintf("binary operation %v is not yet supported", expr.Operation),
		}}
	}

	tokens := leftTokens
	tokens = append(tokens, &hclwrite.Token{
		Type: hclsyntax.TokenIdent, Bytes: []byte(opStr), SpacesBefore: 1,
	})
	if len(rightTokens) > 0 {
		rightTokens[0].SpacesBefore = 1
	}
	tokens = append(tokens, rightTokens...)
	return tokens, diags
}

// unaryOpTokens generates HCL tokens for a unary operation expression.
func unaryOpTokens(expr *model.UnaryOpExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	operandTokens, diags := exprTokens(expr.Operand)
	if diags.HasErrors() {
		return nil, diags
	}

	var opStr string
	switch expr.Operation {
	case hclsyntax.OpLogicalNot:
		opStr = "!"
	case hclsyntax.OpNegate:
		opStr = "-"
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported unary operation",
			Detail:   fmt.Sprintf("unary operation %v is not yet supported", expr.Operation),
		}}
	}

	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(opStr)},
	}
	tokens = append(tokens, operandTokens...)
	return tokens, diags
}
