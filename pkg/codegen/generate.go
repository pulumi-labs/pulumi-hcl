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
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"
)

// GenerateProgram generates HCL source code from a bound PCL program.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	// Create a generator context to track invoke data sources
	gen := &generator{
		program: program,
	}

	genRequiredProviders(body, program)

	// First pass: collect all invoke calls and generate data sources
	for _, node := range program.Nodes {
		gen.collectInvokes(node)
	}

	// Generate data source blocks for invokes
	for _, ds := range gen.invokeDataSources {
		d := gen.genInvokeDataSource(body, ds.expr, ds.name)
		diags = append(diags, d...)
	}

	if len(gen.invokeDataSources) > 0 {
		body.AppendNewline()
	}

	// Second pass: generate resources, outputs, etc.
	for _, node := range program.Nodes {
		switch n := node.(type) {
		case *pcl.Resource:
			d := gen.genResource(body, n)
			diags = append(diags, d...)
		case *pcl.OutputVariable:
			d := gen.genOutput(body, n)
			diags = append(diags, d...)
		case *pcl.ConfigVariable:
			d := gen.genConfigVariable(body, n)
			diags = append(diags, d...)
		case *pcl.LocalVariable:
			d := gen.genLocalVariable(body, n)
			diags = append(diags, d...)
		case *pcl.PulumiBlock:
			d := gen.genPulumiBlock(body, n)
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

// generator holds state during code generation, including invoke data sources.
type generator struct {
	program           *pcl.Program
	invokeDataSources []spilledDataSource
}

type spilledDataSource struct {
	expr *model.FunctionCallExpression
	name string
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

// collectInvokes walks the node and collects all invoke function calls.
func (g *generator) collectInvokes(node pcl.Node) {
	switch n := node.(type) {
	case *pcl.Resource:
		for _, attr := range n.Inputs {
			g.collectInvokesInExpr(attr.Value)
		}
	case *pcl.OutputVariable:
		g.collectInvokesInExpr(n.Value)
	case *pcl.LocalVariable:
		g.collectInvokesInExpr(n.Definition.Value)
	}
}

// collectInvokesInExpr walks an expression tree and collects invoke calls.
func (g *generator) collectInvokesInExpr(expr model.Expression) {
	_, diags := model.VisitExpression(expr, nil, func(expr model.Expression) (model.Expression, hcl.Diagnostics) {
		if call, ok := expr.(*model.FunctionCallExpression); ok {
			if call.Name == pcl.Invoke {
				g.invokeDataSources = append(g.invokeDataSources, spilledDataSource{
					expr: call,
					name: fmt.Sprintf("invoke_%d", len(g.invokeDataSources)),
				})
			}
		}
		return expr, nil
	})
	contract.Assertf(len(diags) == 0, "we never return diags")
}

// genInvokeDataSource generates a data source block for an invoke call.
func (g *generator) genInvokeDataSource(body *hclwrite.Body, invoke *model.FunctionCallExpression, dsName string) hcl.Diagnostics {
	if len(invoke.Args) < 2 {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid invoke call",
			Detail:   "invoke requires at least 2 arguments: token and args",
		}}
	}

	// First arg is the token (function name)
	token, ok := extractStringLiteral(invoke.Args[0])
	if !ok {
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "invalid invoke call",
			Detail:   "invoke token must be a string literal",
		}}
	}

	dsType, diags := tokenToHCLType(token)
	if diags.HasErrors() {
		return diags
	}

	block := body.AppendNewBlock("data", []string{dsType, dsName})

	// Second arg is the args object
	if len(invoke.Args) >= 2 {
		argsExpr := invoke.Args[1]
		if objExpr, ok := argsExpr.(*model.ObjectConsExpression); ok {
			var diags hcl.Diagnostics
			for _, item := range objExpr.Items {
				keyLit, ok := item.Key.(*model.LiteralValueExpression)
				if !ok {
					continue
				}
				keyName := keyLit.Value.AsString()
				d := g.genExpression(block.Body(), keyName, item.Value)
				diags = append(diags, d...)
			}
			return diags
		}
	}

	return nil
}

// tokenToHCLType converts a Pulumi type token to an HCL type name.
// e.g., "aws:index:getAmi" -> "aws_getami"
// e.g., "aws:ec2:getAmi" -> "aws_ec2_getami"
func tokenToHCLType(token string) (string, hcl.Diagnostics) {
	pkg, mod, name, diags := pcl.DecomposeToken(token, hcl.Range{})
	if diags.HasErrors() {
		return "", diags
	}
	// Strip the "index" module (standard Pulumi convention for the default module).
	// Also strip the module when it equals the package name (e.g. pulumi:pulumi:StackReference).
	if mod == "index" || mod == pkg || mod == "" {
		return strings.ToLower(pkg + "_" + name), nil
	}
	return strings.ToLower(pkg + "_" + mod + "_" + name), nil
}

func (g *generator) genResource(body *hclwrite.Body, r *pcl.Resource) hcl.Diagnostics {
	hclType, d := tokenToHCLType(r.Token)
	if d.HasErrors() {
		return d
	}
	block := body.AppendNewBlock("resource", []string{hclType, r.LogicalName()})
	var diags hcl.Diagnostics

	// Handle provider option if present
	if r.Options != nil && r.Options.Provider != nil {
		tokens, d := g.exprTokens(r.Options.Provider)
		diags = append(diags, d...)
		if !d.HasErrors() {
			block.Body().SetAttributeRaw("provider", tokens)
		}
	}

	// Handle additionalSecretOutputs option if present
	if r.Options != nil && r.Options.AdditionalSecretOutputs != nil {
		tokens, d := g.exprTokens(r.Options.AdditionalSecretOutputs)
		diags = append(diags, d...)
		if !d.HasErrors() {
			block.Body().SetAttributeRaw("additional_secret_outputs", tokens)
		}
	}

	for _, attr := range r.Inputs {
		d := g.genExpression(block.Body(), attr.Name, attr.Value)
		diags = append(diags, d...)
	}
	return diags
}

func (g *generator) genConfigVariable(body *hclwrite.Body, cv *pcl.ConfigVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("variable", []string{cv.LogicalName()})

	// Set the type constraint if the config has a type label.
	if len(cv.SyntaxNode().(*hclsyntax.Block).Labels) == 2 {
		typeStr := cv.SyntaxNode().(*hclsyntax.Block).Labels[1]
		hclTypeStr := convertPCLTypeToHCL(typeStr)
		block.Body().SetAttributeRaw("type", hclwrite.Tokens{
			{Type: hclsyntax.TokenIdent, Bytes: []byte(hclTypeStr)},
		})
	}

	// Set the default value if present.
	if cv.DefaultValue != nil {
		tokens, diags := g.exprTokens(cv.DefaultValue)
		if diags.HasErrors() {
			return diags
		}
		block.Body().SetAttributeRaw("default", tokens)
	}

	return nil
}

// convertPCLTypeToHCL converts a PCL type string to an HCL type string.
// The main difference is that PCL uses "int" but HCL uses "number".
// Uses word boundaries to only replace complete "int" tokens, not substrings.
func convertPCLTypeToHCL(pclType string) string {
	// Replace "int" only when it's a complete word/token, not part of another word.
	// This handles cases like "int", "map(int)", "list(int)", "object({prop=list(int)})", etc.
	// while avoiding incorrect replacements like "integer" -> "numberer".
	re := regexp.MustCompile(`\bint\b`)
	return re.ReplaceAllString(pclType, "number")
}

func (g *generator) genLocalVariable(body *hclwrite.Body, lv *pcl.LocalVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("locals", nil)
	return g.genExpression(block.Body(), lv.LogicalName(), lv.Definition.Value)
}

func (g *generator) genOutput(body *hclwrite.Body, ov *pcl.OutputVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("output", []string{ov.LogicalName()})
	return g.genExpression(block.Body(), "value", ov.Value)
}

func (g *generator) genPulumiBlock(body *hclwrite.Body, pb *pcl.PulumiBlock) hcl.Diagnostics {
	if pb.RequiredVersion == nil {
		return nil
	}

	// Generate a top-level "pulumi" block with requiredVersionRange property
	block := body.AppendNewBlock("pulumi", nil)
	return g.genExpression(block.Body(), "requiredVersionRange", pb.RequiredVersion)
}

func (g *generator) genExpression(body *hclwrite.Body, name string, expr model.Expression) hcl.Diagnostics {
	tokens, diags := g.exprTokens(expr)
	if diags.HasErrors() {
		return diags
	}
	body.SetAttributeRaw(name, tokens)
	return diags
}

// exprTokens converts a PCL expression into HCL tokens.
// Invoke calls are replaced with references to generated data sources.
func (g *generator) exprTokens(expr model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	switch e := expr.(type) {
	case *model.LiteralValueExpression:
		return hclwrite.TokensForValue(e.Value), nil
	case *model.TemplateExpression:
		return g.templateTokens(e)
	case *model.FunctionCallExpression:
		// Check if this is an invoke call that we've replaced with a data source
		if e.Name == pcl.Invoke {
			var dsName string
			for _, v := range g.invokeDataSources {
				if v.expr == e {
					dsName = v.name
					break
				}
			}
			if dsName != "" {
				// Generate reference to data source: data.type.name
				token, ok := extractStringLiteral(e.Args[0])
				if !ok {
					return nil, hcl.Diagnostics{{
						Severity: hcl.DiagError,
						Summary:  "invalid invoke call",
						Detail:   "invoke token must be a string literal",
					}}
				}
				dsType, diags := tokenToHCLType(token)
				if diags.HasErrors() {
					return nil, diags
				}
				return hclwrite.TokensForTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "data"},
					hcl.TraverseAttr{Name: dsType},
					hcl.TraverseAttr{Name: dsName},
				}), nil
			}
		}
		return g.funcCallTokens(e)
	case *model.ScopeTraversalExpression:
		return g.scopeTraversalTokens(e)
	case *model.TupleConsExpression:
		return g.tupleTokens(e)
	case *model.ObjectConsExpression:
		return g.objectTokens(e)
	case *model.IndexExpression:
		return g.indexExprTokens(e)
	case *model.RelativeTraversalExpression:
		return g.relativeTraversalTokens(e)
	case *model.BinaryOpExpression:
		return g.binaryOpTokens(e)
	case *model.UnaryOpExpression:
		return g.unaryOpTokens(e)
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	}
}

func (g *generator) funcCallTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
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
		return g.getOutputTokens(expr)
	case "secret":
		return g.passthroughFuncCallTokens("sensitive", expr.Args)
	default:
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}
}

// getOutputTokens generates tokens for getOutput(resource, "outputName").
// This produces resource_type.name.outputs["outputName"].
func (g *generator) getOutputTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
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
	hclType, diags := tokenToHCLType(res.Token)
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
// PCL config variables become HCL `var.<name>`, local variables become `local.<name>`,
// and resource references become `<resource_type>.<name>.<property>`.
func (g *generator) scopeTraversalTokens(expr *model.ScopeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	traversal := expr.Traversal
	if len(expr.Parts) > 0 {
		switch part := expr.Parts[0].(type) {
		case *pcl.ConfigVariable:
			// Rewrite "aMap.x" → "var.aMap.x".
			rewritten := make(hcl.Traversal, 0, len(traversal)+1)
			rewritten = append(rewritten, hcl.TraverseRoot{Name: "var"})
			rewritten = append(rewritten, hcl.TraverseAttr{Name: traversal.RootName()})
			rewritten = append(rewritten, traversal[1:]...)
			traversal = rewritten
		case *pcl.LocalVariable:
			// Rewrite "myLocal.x" → "local.myLocal.x".
			rewritten := make(hcl.Traversal, 0, len(traversal)+1)
			rewritten = append(rewritten, hcl.TraverseRoot{Name: "local"})
			rewritten = append(rewritten, hcl.TraverseAttr{Name: traversal.RootName()})
			rewritten = append(rewritten, traversal[1:]...)
			traversal = rewritten
		case *pcl.Resource:
			// Rewrite "myResource.property" → "resource_type.myResource.property".
			hclType, diags := tokenToHCLType(part.Token)
			if diags.HasErrors() {
				return nil, diags
			}
			rewritten := make(hcl.Traversal, 0, len(traversal)+1)
			rewritten = append(rewritten, hcl.TraverseRoot{Name: hclType})
			rewritten = append(rewritten, hcl.TraverseAttr{Name: traversal.RootName()})
			rewritten = append(rewritten, traversal[1:]...)
			traversal = rewritten
		}
	}
	return hclwrite.TokensForTraversal(traversal), nil
}

// passthroughFuncCallTokens generates tokens for a function call: name(arg1, arg2, ...).
func (g *generator) passthroughFuncCallTokens(name string, args []model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte(name)},
		{Type: hclsyntax.TokenOParen, Bytes: []byte("(")},
	}
	var diags hcl.Diagnostics
	for i, arg := range args {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		argTokens, d := g.exprTokens(arg)
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
func (g *generator) indexExprTokens(expr *model.IndexExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	collTokens, diags := g.exprTokens(expr.Collection)
	if diags.HasErrors() {
		return nil, diags
	}
	keyTokens, d := g.exprTokens(expr.Key)
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
func (g *generator) relativeTraversalTokens(expr *model.RelativeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	sourceTokens, diags := g.exprTokens(expr.Source)
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

func (g *generator) tupleTokens(expr *model.TupleConsExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
	}
	var diags hcl.Diagnostics
	for i, elem := range expr.Expressions {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		elemTokens, d := g.exprTokens(elem)
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

func (g *generator) objectTokens(expr *model.ObjectConsExpression) (hclwrite.Tokens, hcl.Diagnostics) {
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
		keyTokens, d := g.exprTokens(item.Key)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		valTokens, d := g.exprTokens(item.Value)
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
func (g *generator) templateTokens(expr *model.TemplateExpression) (hclwrite.Tokens, hcl.Diagnostics) {
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
				fmt.Fprintf(&buf, "${%s}", p.Value.GoString())
			}
		default:
			partTokens, d := g.exprTokens(part)
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
func (g *generator) binaryOpTokens(expr *model.BinaryOpExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	leftTokens, diags := g.exprTokens(expr.LeftOperand)
	if diags.HasErrors() {
		return nil, diags
	}

	rightTokens, d := g.exprTokens(expr.RightOperand)
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
func (g *generator) unaryOpTokens(expr *model.UnaryOpExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	operandTokens, diags := g.exprTokens(expr.Operand)
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
