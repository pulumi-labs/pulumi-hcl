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
	"path/filepath"
	"regexp"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/eval"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/hcl2/model"
	"github.com/pulumi/pulumi/pkg/v3/codegen/pcl"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/tokens"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/zclconf/go-cty/cty"
)

// GenerateProgram generates HCL source code from a bound PCL program.
func GenerateProgram(program *pcl.Program) (map[string][]byte, hcl.Diagnostics, error) {
	var diags hcl.Diagnostics
	f := hclwrite.NewEmptyFile()
	body := f.Body()

	// Create a generator context to track invoke data sources and call blocks
	gen := &generator{
		program: program,
	}

	genRequiredProviders(body, program)

	// First pass: collect all invoke calls and call expressions
	for _, node := range program.Nodes {
		gen.collectInvokes(node)
		gen.collectCalls(node)
	}

	// Generate data source blocks for invokes
	for _, ds := range gen.invokeDataSources {
		d := gen.genInvokeDataSource(body, ds.expr, ds.name)
		diags = append(diags, d...)
	}

	if len(gen.invokeDataSources) > 0 {
		body.AppendNewline()
	}

	// Generate call blocks
	for _, cb := range gen.callBlocks {
		d := gen.genCallBlock(body, cb)
		diags = append(diags, d...)
	}

	if len(gen.callBlocks) > 0 {
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
		case *pcl.Component:
			d := gen.genModule(body, n)
			diags = append(diags, d...)
		default:
			diags = append(diags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "unsupported PCL node type",
				Detail:   fmt.Sprintf("node type %T is not yet supported", node),
			})
		}
	}

	files := map[string][]byte{"main.hcl": f.Bytes()}
	for componentDir, component := range program.CollectComponents() {
		subFiles, d, err := GenerateProgram(component.Program)
		diags = append(diags, d...)
		if err != nil {
			return nil, diags, err
		}
		subDirName := filepath.Base(componentDir)
		for name, content := range subFiles {
			files[filepath.Join(subDirName, name)] = content
		}
	}
	return files, diags, nil
}

type rangeKind int

const (
	rangeKindNone    rangeKind = iota
	rangeKindCount             // bool/number → count
	rangeKindForEach           // list/map → for_each
)

// dynamicBlockContext tracks the iterator variables of a dynamic block so that
// references to them can be rewritten in scopeTraversalTokens.
type dynamicBlockContext struct {
	blockName     string
	keyVariable   *model.Variable
	valueVariable *model.Variable
}

// generator holds state during code generation, including invoke data sources.
type generator struct {
	program           *pcl.Program
	invokeDataSources []spilledDataSource
	callBlocks        []spilledCall
	currentRangeKind  rangeKind
	dynamicBlock      *dynamicBlockContext
}

type spilledDataSource struct {
	expr *model.FunctionCallExpression
	name string
}

type spilledCall struct {
	expr         *model.FunctionCallExpression
	resourceName string
	methodName   string
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
		namespace := ref.Namespace()
		if namespace == "" {
			namespace = "pulumi"
		}
		attrs := map[string]cty.Value{
			"source": cty.StringVal(namespace + "/" + ref.Name()),
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

// collectCalls walks the node and collects all call function expressions.
func (g *generator) collectCalls(node pcl.Node) {
	switch n := node.(type) {
	case *pcl.Resource:
		for _, attr := range n.Inputs {
			g.collectCallsInExpr(attr.Value)
		}
	case *pcl.OutputVariable:
		g.collectCallsInExpr(n.Value)
	case *pcl.LocalVariable:
		g.collectCallsInExpr(n.Definition.Value)
	}
}

// collectCallsInExpr walks an expression tree and collects call expressions.
func (g *generator) collectCallsInExpr(expr model.Expression) {
	_, diags := model.VisitExpression(expr, nil, func(expr model.Expression) (model.Expression, hcl.Diagnostics) {
		if call, ok := expr.(*model.FunctionCallExpression); ok {
			if call.Name == pcl.Call && len(call.Args) >= 2 {
				resourceName, methodName, ok := g.extractCallArgs(call)
				if ok {
					snakeMethod := transform.SnakeCaseFromPulumiCase(methodName)
					// Deduplicate: only add if we haven't already seen this resourceName.methodName
					duplicate := false
					for _, cb := range g.callBlocks {
						if cb.resourceName == resourceName && cb.methodName == snakeMethod {
							duplicate = true
							break
						}
					}
					if !duplicate {
						g.callBlocks = append(g.callBlocks, spilledCall{
							expr:         call,
							resourceName: resourceName,
							methodName:   snakeMethod,
						})
					}
				}
			}
		}
		return expr, nil
	})
	contract.Assertf(len(diags) == 0, "we never return diags")
}

// extractCallArgs extracts (resourceName, methodName) from a pcl.Call expression.
// Returns ok=false if the args are not the expected form.
func (g *generator) extractCallArgs(call *model.FunctionCallExpression) (resourceName, methodName string, ok bool) {
	if len(call.Args) < 2 {
		return "", "", false
	}
	// First arg: resource reference
	scopeTraversal, isScopeTraversal := call.Args[0].(*model.ScopeTraversalExpression)
	if !isScopeTraversal || len(scopeTraversal.Parts) == 0 {
		return "", "", false
	}
	switch part := scopeTraversal.Parts[0].(type) {
	case *pcl.Resource:
		resourceName = part.LogicalName()
	default:
		// Could be a provider - use the traversal root name
		resourceName = scopeTraversal.Traversal.RootName()
	}
	// Second arg: method name string literal
	methodName, ok = extractStringLiteralFromCallArg(call.Args[1])
	return resourceName, methodName, ok
}

// extractStringLiteralFromCallArg extracts a string value from a method name argument,
// which can be a TemplateExpression or a LiteralValueExpression.
func extractStringLiteralFromCallArg(expr model.Expression) (string, bool) {
	switch e := expr.(type) {
	case *model.LiteralValueExpression:
		if e.Value.Type() == cty.String {
			return e.Value.AsString(), true
		}
	case *model.TemplateExpression:
		if len(e.Parts) == 1 {
			if lit, ok := e.Parts[0].(*model.LiteralValueExpression); ok && lit.Value.Type() == cty.String {
				return lit.Value.AsString(), true
			}
		}
	}
	return "", false
}

// genCallBlock generates a call block for a method invocation.
func (g *generator) genCallBlock(body *hclwrite.Body, cb spilledCall) hcl.Diagnostics {
	block := body.AppendNewBlock("call", []string{cb.resourceName, cb.methodName})

	if len(cb.expr.Args) < 3 {
		return nil
	}

	var diags hcl.Diagnostics
	argsExpr := cb.expr.Args[2]
	if objExpr, ok := argsExpr.(*model.ObjectConsExpression); ok {
		for _, item := range objExpr.Items {
			keyLit, ok := item.Key.(*model.LiteralValueExpression)
			if !ok {
				continue
			}
			keyName := keyLit.Value.AsString()
			hclName := transform.SnakeCaseFromPulumiCase(keyName)
			d := g.genExpression(block.Body(), hclName, item.Value, schema.AnyType)
			diags = append(diags, d...)
		}
	}

	return diags
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

	var invokeSchema *schema.Function
	for _, p := range g.program.PackageReferences() {
		if p.Name() == tokens.Type(token).Package().String() {
			pkg, mod, name, _ := pcl.DecomposeToken(token, hcl.Range{})
			// PCL normalizes "pkg:index:name" to "pkg::name", but schema
			// stores the original token. Reconstruct the canonical form
			// using DecomposeToken (which fills in "index" for an empty module)
			// and retry.
			token = pkg + ":" + mod + ":" + name
			f, ok, err := p.Functions().Get(token)
			if err != nil {
				return hcl.Diagnostics{{
					Severity: hcl.DiagError,
					Summary:  "failed to get invoke " + token,
					Detail:   err.Error(),
				}}
			}
			if ok {
				invokeSchema = f
			}
			break
		}
	}

	dsType, diags := packages.PulumiTokenToHCL(token)
	if diags.HasErrors() {
		return diags
	}

	block := body.AppendNewBlock("data", []string{dsType, dsName})

	// Second arg is the args object
	if len(invoke.Args) >= 2 {
		argsExpr := invoke.Args[1]
		if objExpr, ok := argsExpr.(*model.ObjectConsExpression); ok {
			for _, item := range objExpr.Items {
				keyLit, ok := item.Key.(*model.LiteralValueExpression)
				if !ok {
					continue
				}
				keyName := keyLit.Value.AsString()
				var propType schema.Type
				if invokeSchema != nil && invokeSchema.Inputs != nil {
					if inputSchema, ok := invokeSchema.Inputs.Property(keyName); ok {
						propType = inputSchema.Type
					}
				}
				hclName := transform.SnakeCaseFromPulumiCase(keyName)
				if objType, ok := transform.AsHCLBlockType(propType); ok {
					d := g.genBlocks(block.Body(), hclName, item.Value, objType)
					diags = append(diags, d...)
				} else {
					d := g.genExpression(block.Body(), hclName, item.Value, propType)
					diags = append(diags, d...)
				}
			}
		}
	}

	// Third arg is the invoke options object
	if len(invoke.Args) >= 3 {
		if optsExpr, ok := invoke.Args[2].(*model.ObjectConsExpression); ok {
			for _, item := range optsExpr.Items {
				keyLit, ok := item.Key.(*model.LiteralValueExpression)
				if !ok {
					continue
				}
				switch keyLit.Value.AsString() {
				case "provider", "parent":
					tokens, d := g.exprTokens(item.Value, schema.AnyType)
					diags = append(diags, d...)
					if !d.HasErrors() {
						block.Body().SetAttributeRaw(keyLit.Value.AsString(), tokens)
					}
				case "version":
					tokens, d := g.exprTokens(item.Value, schema.StringType)
					diags = append(diags, d...)
					if !d.HasErrors() {
						block.Body().SetAttributeRaw("version", tokens)
					}
				case "pluginDownloadUrl":
					tokens, d := g.exprTokens(item.Value, schema.StringType)
					diags = append(diags, d...)
					if !d.HasErrors() {
						block.Body().SetAttributeRaw("plugin_download_url", tokens)
					}
				case "dependsOn":
					tokens, d := g.exprTokens(item.Value, schema.AnyType)
					diags = append(diags, d...)
					if !d.HasErrors() {
						block.Body().SetAttributeRaw("depends_on", tokens)
					}
				}
			}
		}
	}

	return diags
}

func (g *generator) genResource(body *hclwrite.Body, r *pcl.Resource) hcl.Diagnostics {
	defer func() { g.currentRangeKind = rangeKindNone }()

	hclType, d := packages.PulumiTokenToHCL(r.Token)
	if d.HasErrors() {
		return d
	}
	block := body.AppendNewBlock("resource", []string{hclType, r.LogicalName()})
	var diags hcl.Diagnostics

	d = g.genResourceOptions(block.Body(), r)
	diags = append(diags, d...)

	var inputs []*schema.Property
	if r.Schema != nil {
		inputs = r.Schema.InputProperties
	}

	for _, attr := range r.Inputs {
		var inputType schema.Type
		for _, prop := range inputs {
			if attr.Name == prop.Name {
				inputType = prop.Type
			}
		}
		hclName := transform.SnakeCaseFromPulumiCase(attr.Name)
		if objType, ok := transform.AsHCLBlockType(inputType); ok {
			d := g.genBlocks(block.Body(), hclName, attr.Value, objType)
			diags = append(diags, d...)
		} else {
			d := g.genExpression(block.Body(), hclName, attr.Value, inputType)
			diags = append(diags, d...)
		}
	}
	return diags
}

// genResourceOptions generates HCL meta-arguments for a resource's options.
func (g *generator) genResourceOptions(body *hclwrite.Body, r *pcl.Resource) hcl.Diagnostics {
	var diags hcl.Diagnostics
	opts := r.Options

	// Collect schema-based replaceOnChanges property paths in camelCase (Pulumi convention).
	var schemaReplaceOnChanges []string
	if r.Schema != nil {
		schemaReplaceProps, _ := r.Schema.ReplaceOnChanges()
		// Keep property names in camelCase so the engine can match them against the diff.
		schemaReplaceOnChanges = schema.PropertyListJoinToString(schemaReplaceProps,
			func(s string) string { return s })
	}

	if opts == nil && len(schemaReplaceOnChanges) == 0 {
		return nil
	}

	if opts == nil {
		// Only schema-based replaceOnChanges - generate it
		g.genReplaceOnChanges(body, schemaReplaceOnChanges, nil, &diags)
		return diags
	}

	if opts.Range != nil {
		d := g.genRange(body, opts.Range)
		diags = append(diags, d...)
	}

	if opts.Parent != nil {
		tokens, d := g.exprTokens(opts.Parent, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("parent", tokens)
		}
	}

	if opts.Provider != nil {
		tokens, d := g.exprTokens(opts.Provider, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("provider", tokens)
		}
	}

	if opts.Providers != nil {
		tokens, d := g.genProvidersTokens(opts.Providers)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("providers", tokens)
		}
	}

	if opts.AdditionalSecretOutputs != nil {
		g.genPropertyPathList(body, "additional_secret_outputs", opts.AdditionalSecretOutputs, &diags)
	}

	if opts.RetainOnDelete != nil {
		tokens, d := g.exprTokens(opts.RetainOnDelete, schema.BoolType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("retain_on_delete", tokens)
		}
	}

	if opts.DeletedWith != nil {
		tokens, d := g.exprTokens(opts.DeletedWith, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("deleted_with", tokens)
		}
	}

	if opts.ReplaceWith != nil {
		tokens, d := g.exprTokens(opts.ReplaceWith, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("replace_with", tokens)
		}
	}

	if opts.HideDiffs != nil {
		g.genPropertyPathList(body, "hide_diffs", opts.HideDiffs, &diags)
	}

	g.genReplaceOnChanges(body, schemaReplaceOnChanges, opts.ReplaceOnChanges, &diags)

	if opts.ReplacementTrigger != nil {
		tokens, d := g.exprTokens(opts.ReplacementTrigger, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("replacement_trigger", tokens)
		}
	}

	if opts.IgnoreChanges != nil || opts.Protect != nil || opts.DeleteBeforeReplace != nil {
		lifecycleBlock := body.AppendNewBlock("lifecycle", nil)
		if opts.Protect != nil {
			tokens, d := g.exprTokens(opts.Protect, schema.BoolType)
			diags = append(diags, d...)
			if !d.HasErrors() {
				lifecycleBlock.Body().SetAttributeRaw("prevent_destroy", tokens)
			}
		}
		if opts.IgnoreChanges != nil {
			tokens, d := g.exprTokens(opts.IgnoreChanges, schema.AnyType)
			diags = append(diags, d...)
			if !d.HasErrors() {
				lifecycleBlock.Body().SetAttributeRaw("ignore_changes", tokens)
			}
		}
		if opts.DeleteBeforeReplace != nil {
			// PCL deleteBeforeReplace=true means delete-then-create.
			// HCL create_before_destroy=true means create-then-delete.
			// So create_before_destroy = !deleteBeforeReplace.
			// We generate the inverted expression.
			tokens, d := g.exprTokens(opts.DeleteBeforeReplace, schema.BoolType)
			diags = append(diags, d...)
			if !d.HasErrors() {
				invertedTokens := append(hclwrite.Tokens{
					{Type: hclsyntax.TokenBang, Bytes: []byte("!")},
				}, tokens...)
				lifecycleBlock.Body().SetAttributeRaw("create_before_destroy", invertedTokens)
			}
		}
	}

	if opts.CustomTimeouts != nil {
		timeoutsBlock := body.AppendNewBlock("timeouts", nil)
		if obj, ok := opts.CustomTimeouts.(*model.ObjectConsExpression); ok {
			for _, item := range obj.Items {
				keyName, ok := extractStringLiteral(item.Key)
				if !ok {
					continue
				}
				hclName := strings.ToLower(keyName)
				tokens, d := g.exprTokens(item.Value, schema.StringType)
				diags = append(diags, d...)
				if !d.HasErrors() {
					timeoutsBlock.Body().SetAttributeRaw(hclName, tokens)
				}
			}
		}
	}

	if opts.DependsOn != nil {
		tokens, d := g.exprTokens(opts.DependsOn, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("depends_on", tokens)
		}
	}

	if opts.ImportID != nil {
		tokens, d := g.exprTokens(opts.ImportID, schema.StringType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("import_id", tokens)
		}
	}

	if opts.EnvVarMappings != nil {
		tokens, d := g.exprTokens(opts.EnvVarMappings, schema.AnyType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("env_var_mappings", tokens)
		}
	}

	// HCL doesn't bake versions into generated code, so always emit version when specified.
	if opts.Version != nil {
		tokens, d := g.exprTokens(opts.Version, schema.StringType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("version", tokens)
		}
	}

	if opts.PluginDownloadURL != nil && pcl.NeedsPluginDownloadURLResourceOption(opts.PluginDownloadURL, r.Schema) {
		tokens, d := g.exprTokens(opts.PluginDownloadURL, schema.StringType)
		diags = append(diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("plugin_download_url", tokens)
		}
	}

	if opts.Aliases != nil {
		g.genAliases(body, opts.Aliases, &diags)
	}

	return diags
}

// genRange emits a count or for_each meta-argument based on the PCL range expression type.
func (g *generator) genRange(body *hclwrite.Body, rangeExpr model.Expression) hcl.Diagnostics {
	rangeType := model.ResolveOutputs(rangeExpr.Type())

	switch {
	case model.InputType(model.BoolType).ConversionFrom(rangeType) == model.SafeConversion:
		tokens, d := g.exprTokens(rangeExpr, schema.AnyType)
		if d.HasErrors() {
			return d
		}
		body.SetAttributeRaw("count", tokens)
		g.currentRangeKind = rangeKindCount

	case model.InputType(model.NumberType).ConversionFrom(rangeType) == model.SafeConversion:
		tokens, d := g.exprTokens(rangeExpr, schema.AnyType)
		if d.HasErrors() {
			return d
		}
		body.SetAttributeRaw("count", tokens)
		g.currentRangeKind = rangeKindCount

	default:
		exprTokens, d := g.exprTokens(rangeExpr, schema.AnyType)
		if d.HasErrors() {
			return d
		}
		var tokens hclwrite.Tokens
		switch rangeType.(type) {
		case *model.ListType, *model.TupleType:
			tokens = wrapListAsMapForEach(exprTokens)
		default:
			tokens = exprTokens
		}
		body.SetAttributeRaw("for_each", tokens)
		g.currentRangeKind = rangeKindForEach
	}
	return nil
}

// wrapListAsMapForEach generates `{ for __key, __value in <expr> : tostring(__key) => __value }`.
func wrapListAsMapForEach(listTokens hclwrite.Tokens) hclwrite.Tokens {
	tok := func(t hclsyntax.TokenType, s string) *hclwrite.Token {
		return &hclwrite.Token{Type: t, Bytes: []byte(s)}
	}
	tokens := hclwrite.Tokens{
		tok(hclsyntax.TokenOBrace, "{"),
		tok(hclsyntax.TokenIdent, " for"),
		tok(hclsyntax.TokenIdent, " __key"),
		tok(hclsyntax.TokenComma, ","),
		tok(hclsyntax.TokenIdent, " __value"),
		tok(hclsyntax.TokenIdent, " in "),
	}
	tokens = append(tokens, listTokens...)
	tokens = append(tokens,
		tok(hclsyntax.TokenColon, " :"),
		tok(hclsyntax.TokenIdent, " tostring"),
		tok(hclsyntax.TokenOParen, "("),
		tok(hclsyntax.TokenIdent, "__key"),
		tok(hclsyntax.TokenCParen, ")"),
		tok(hclsyntax.TokenFatArrow, " =>"),
		tok(hclsyntax.TokenIdent, " __value"),
		tok(hclsyntax.TokenCBrace, " }"),
	)
	return tokens
}

// genProviders generates the HCL `providers` attribute as a list.
// PCL providers can be a list [p1, p2] or a map {pkg = p}; we always emit a list
// since the package name is recoverable from the provider resource type at runtime.
func (g *generator) genProvidersTokens(providers model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	if providers, ok := providers.(*model.ObjectConsExpression); ok {
		elems := make([]model.Expression, 0, len(providers.Items))
		for _, v := range providers.Items {
			elems = append(elems, v.Value)
		}
		return g.exprTokens(&model.TupleConsExpression{
			Expressions: elems,
		}, &schema.ArrayType{ElementType: schema.AnyResourceType})
	}

	return g.exprTokens(providers, &schema.ArrayType{ElementType: schema.AnyResourceType})
}

// genAliases generates the HCL `aliases` attribute from a PCL aliases expression.
// PCL aliases can be URN strings or spec objects with fields like name, noParent, parent.
// HCL uses snake_case keys (no_parent, parent_urn) and parent is a resource URN string.
func (g *generator) genAliases(body *hclwrite.Body, aliases model.Expression, diags *hcl.Diagnostics) {
	tuple, ok := aliases.(*model.TupleConsExpression)
	if !ok {
		// Fallback: emit as-is
		t, d := g.exprTokens(aliases, schema.AnyType)
		*diags = append(*diags, d...)
		if !d.HasErrors() {
			body.SetAttributeRaw("aliases", t)
		}
		return
	}

	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
	}
	for i, elem := range tuple.Expressions {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		elemTokens, d := g.aliasElemTokens(elem)
		*diags = append(*diags, d...)
		if d.HasErrors() {
			return
		}
		if len(elemTokens) > 0 {
			elemTokens[0].SpacesBefore = 1
		}
		tokens = append(tokens, elemTokens...)
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	body.SetAttributeRaw("aliases", tokens)
}

// aliasElemTokens generates HCL tokens for a single alias element.
// String elements are emitted as-is. Object elements have their keys renamed:
// noParent → no_parent, parent (resource ref) → parent_urn (resource URN string).
func (g *generator) aliasElemTokens(elem model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
	obj, ok := elem.(*model.ObjectConsExpression)
	if !ok {
		return g.exprTokens(elem, schema.AnyType)
	}

	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")},
	}
	if len(obj.Items) == 0 {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})
		return tokens, nil
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	var diags hcl.Diagnostics
	for _, item := range obj.Items {
		keyStr, _ := extractStringLiteral(item.Key)

		var hclKey string
		var valTokens hclwrite.Tokens
		var d hcl.Diagnostics

		switch keyStr {
		case "noParent":
			hclKey = "no_parent"
			valTokens, d = g.exprTokens(item.Value, schema.BoolType)
		case "parent":
			// Transform resource reference to parent_urn = resource_type.name.urn
			hclKey = "parent_urn"
			baseTokens, d2 := g.exprTokens(item.Value, schema.AnyType)
			diags = append(diags, d2...)
			if d2.HasErrors() {
				return nil, diags
			}
			// Append .urn to get the resource's URN
			valTokens = append(baseTokens,
				&hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
				&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("urn")},
			)
		default:
			hclKey = transform.SnakeCaseFromPulumiCase(keyStr)
			valTokens, d = g.exprTokens(item.Value, schema.AnyType)
		}

		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}

		keyTokens := hclwrite.TokensForIdentifier(hclKey)
		keyTokens[0].SpacesBefore = 2
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

// extractPropertyNames extracts camelCase property names from a PCL expression like [value, otherProp].
func extractPropertyNames(expr model.Expression) []string {
	var names []string
	if expr == nil {
		return names
	}
	tuple, ok := expr.(*model.TupleConsExpression)
	if !ok {
		return names
	}
	for _, elem := range tuple.Expressions {
		if s, ok := extractStringLiteral(elem); ok {
			names = append(names, s)
		} else if traversal, ok := elem.(*model.ScopeTraversalExpression); ok {
			// PCL property traversals are already in camelCase (e.g., value, replaceProp)
			names = append(names, traversal.Traversal.RootName())
		}
	}
	return names
}

// genPropertyPathList generates an HCL string list attribute for property paths.
// Property names are emitted as string literals in camelCase (e.g., replace_on_changes = ["replaceProp"]).
func (g *generator) genPropertyPathList(body *hclwrite.Body, attrName string, optsExpr model.Expression, diags *hcl.Diagnostics) {
	names := extractPropertyNames(optsExpr)
	if len(names) == 0 {
		return
	}
	body.SetAttributeRaw(attrName, makeStringListTokens(names))
}

// makeStringListTokens generates HCL tokens for a list of string literals: ["a", "b", "c"].
func makeStringListTokens(strs []string) hclwrite.Tokens {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
	}
	for i, s := range strs {
		if i > 0 {
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")})
		}
		tokens = append(tokens, &hclwrite.Token{
			Type: hclsyntax.TokenQuotedLit, Bytes: []byte(`"` + s + `"`), SpacesBefore: 1,
		})
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
	return tokens
}

// genReplaceOnChanges generates the replace_on_changes attribute, merging schema-based and option-based paths.
// All paths are emitted as string literals in camelCase so the engine can match them against the diff.
func (g *generator) genReplaceOnChanges(body *hclwrite.Body, schemaPaths []string, optsExpr model.Expression, diags *hcl.Diagnostics) {
	optPaths := extractPropertyNames(optsExpr)
	if len(schemaPaths) == 0 && len(optPaths) == 0 {
		return
	}

	// Merge and deduplicate paths.
	seen := make(map[string]bool)
	var allPaths []string
	for _, p := range schemaPaths {
		if !seen[p] {
			seen[p] = true
			allPaths = append(allPaths, p)
		}
	}
	for _, p := range optPaths {
		if !seen[p] {
			seen[p] = true
			allPaths = append(allPaths, p)
		}
	}

	body.SetAttributeRaw("replace_on_changes", makeStringListTokens(allPaths))
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
		tokens, diags := g.exprTokens(cv.DefaultValue, schema.AnyType)
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
	if g.isInvokeLocal(lv) {
		return nil
	}
	block := body.AppendNewBlock("locals", nil)
	return g.genExpression(block.Body(), lv.LogicalName(), lv.Definition.Value, schema.AnyType)
}

// isInvokeLocal reports whether a local variable's definition is an invoke call
// that has been promoted to a data source block.
func (g *generator) isInvokeLocal(lv *pcl.LocalVariable) bool {
	call, ok := lv.Definition.Value.(*model.FunctionCallExpression)
	if !ok || call.Name != pcl.Invoke {
		return false
	}
	for _, ds := range g.invokeDataSources {
		if ds.expr == call {
			return true
		}
	}
	return false
}

func (g *generator) genOutput(body *hclwrite.Body, ov *pcl.OutputVariable) hcl.Diagnostics {
	block := body.AppendNewBlock("output", []string{ov.LogicalName()})
	return g.genExpression(block.Body(), "value", ov.Value, schema.AnyType)
}

func (g *generator) genModule(body *hclwrite.Body, c *pcl.Component) hcl.Diagnostics {
	defer func() { g.currentRangeKind = rangeKindNone }()

	block := body.AppendNewBlock("module", []string{c.LogicalName()})
	source := "./" + filepath.Base(c.DirPath())
	block.Body().SetAttributeValue("source", cty.StringVal(source))
	var diags hcl.Diagnostics

	if c.Options != nil && c.Options.Range != nil {
		d := g.genRange(block.Body(), c.Options.Range)
		diags = append(diags, d...)
	}

	for _, attr := range c.Inputs {
		d := g.genExpression(block.Body(), attr.Name, attr.Value, schema.AnyType)
		diags = append(diags, d...)
	}
	return diags
}

func (g *generator) genPulumiBlock(body *hclwrite.Body, pb *pcl.PulumiBlock) hcl.Diagnostics {
	if pb.RequiredVersion == nil {
		return nil
	}

	// Generate a top-level "pulumi" block with requiredVersionRange property
	block := body.AppendNewBlock("pulumi", nil)
	return g.genExpression(block.Body(), "required_version_range", pb.RequiredVersion, schema.StringType)
}

func (g *generator) genBlocks(body *hclwrite.Body, name string, expr model.Expression, objType *schema.ObjectType) hcl.Diagnostics {
	switch e := expr.(type) {
	case *model.TupleConsExpression:
		var diags hcl.Diagnostics
		for _, elem := range e.Expressions {
			d := g.genBlock(body, name, elem, objType)
			diags = append(diags, d...)
		}
		return diags
	case *model.ForExpression:
		return g.genDynamicBlock(body, name, e, objType)
	default:
		return g.genBlock(body, name, expr, objType)
	}
}

// genDynamicBlock generates a dynamic block from a PCL ForExpression.
//
//	dynamic "name" {
//	  for_each = <collection>
//	  content {
//	    <fields from Value expression, with iterator vars rewritten>
//	  }
//	}
func (g *generator) genDynamicBlock(
	body *hclwrite.Body, name string, expr *model.ForExpression, objType *schema.ObjectType,
) hcl.Diagnostics {
	block := body.AppendNewBlock("dynamic", []string{name})

	collTokens, diags := g.exprTokens(expr.Collection, schema.AnyType)
	if diags.HasErrors() {
		return diags
	}
	block.Body().SetAttributeRaw("for_each", collTokens)

	prev := g.dynamicBlock
	g.dynamicBlock = &dynamicBlockContext{
		blockName:     name,
		keyVariable:   expr.KeyVariable,
		valueVariable: expr.ValueVariable,
	}
	defer func() { g.dynamicBlock = prev }()

	d := g.genBlock(block.Body(), "content", expr.Value, objType)
	diags = append(diags, d...)
	return diags
}

func (g *generator) genBlock(body *hclwrite.Body, name string, expr model.Expression, objType *schema.ObjectType) hcl.Diagnostics {
	block := body.AppendNewBlock(name, nil)
	obj, ok := expr.(*model.ObjectConsExpression)
	if !ok {
		return g.genExpression(block.Body(), "content", expr, objType)
	}
	var diags hcl.Diagnostics
	for _, item := range obj.Items {
		keyName, _ := extractStringLiteral(item.Key)
		snakeName := transform.SnakeCaseFromPulumiCase(keyName)
		propType := schema.AnyType
		if p, ok := objType.Property(keyName); ok {
			propType = p.Type
		}
		d := g.genExpression(block.Body(), snakeName, item.Value, propType)
		diags = append(diags, d...)
	}
	return diags
}

func (g *generator) genExpression(body *hclwrite.Body, name string, expr model.Expression, typ schema.Type) hcl.Diagnostics {
	tokens, diags := g.exprTokens(expr, typ)
	if diags.HasErrors() {
		return diags
	}
	body.SetAttributeRaw(name, tokens)
	return diags
}

// exprTokens converts a PCL expression into HCL tokens.
// Invoke calls are replaced with references to generated data sources.
func (g *generator) exprTokens(expr model.Expression, typ schema.Type) (hclwrite.Tokens, hcl.Diagnostics) {
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
				dsType, diags := packages.PulumiTokenToHCL(token)
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
		// Check if this is a call expression that we've replaced with a call block
		if e.Name == pcl.Call {
			resourceName, methodName, ok := g.extractCallArgs(e)
			if ok {
				snakeMethod := transform.SnakeCaseFromPulumiCase(methodName)
				return hclwrite.TokensForTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "call"},
					hcl.TraverseAttr{Name: resourceName},
					hcl.TraverseAttr{Name: snakeMethod},
				}), nil
			}
		}
		return g.funcCallTokens(e)
	case *model.ScopeTraversalExpression:
		return g.scopeTraversalTokens(e)
	case *model.TupleConsExpression:
		return g.tupleTokens(e)
	case *model.ObjectConsExpression:
		return g.objectTokens(e, typ)
	case *model.IndexExpression:
		return g.indexExprTokens(e)
	case *model.RelativeTraversalExpression:
		return g.relativeTraversalTokens(e)
	case *model.BinaryOpExpression:
		return g.binaryOpTokens(e)
	case *model.UnaryOpExpression:
		return g.unaryOpTokens(e)
	case *model.ForExpression:
		return g.forExprTokens(e)
	case *model.SplatExpression:
		return g.splatTokens(e)
	case *model.ConditionalExpression:
		return g.conditionalTokens(e)
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	}
}

// forExprTokens generates HCL tokens for a PCL ForExpression.
//
// List result (Key == nil):  [for key, value in collection : valueExpr]
// Map result (Key != nil):   {for key, value in collection : keyExpr => valueExpr}
func (g *generator) forExprTokens(expr *model.ForExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	var diags hcl.Diagnostics

	collTokens, d := g.exprTokens(expr.Collection, schema.AnyType)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	valueTokens, d := g.exprTokens(expr.Value, schema.AnyType)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	isMap := expr.Key != nil

	var open, close byte
	if isMap {
		open, close = '{', '}'
	} else {
		open, close = '[', ']'
	}

	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenTemplateControl, Bytes: []byte{open}},
		{Type: hclsyntax.TokenIdent, Bytes: []byte("for"), SpacesBefore: 0},
	}

	if expr.KeyVariable != nil {
		tokens = append(tokens,
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(expr.KeyVariable.Name), SpacesBefore: 1},
			&hclwrite.Token{Type: hclsyntax.TokenComma, Bytes: []byte(",")},
		)
	}

	tokens = append(tokens,
		&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(expr.ValueVariable.Name), SpacesBefore: 1},
		&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("in"), SpacesBefore: 1},
	)

	if len(collTokens) > 0 {
		collTokens[0].SpacesBefore = 1
	}
	tokens = append(tokens, collTokens...)
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenColon, Bytes: []byte(":"), SpacesBefore: 1})

	if isMap {
		keyTokens, d := g.exprTokens(expr.Key, schema.AnyType)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		if len(keyTokens) > 0 {
			keyTokens[0].SpacesBefore = 1
		}
		tokens = append(tokens, keyTokens...)
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenFatArrow, Bytes: []byte("=>"), SpacesBefore: 1})
	}

	if len(valueTokens) > 0 {
		valueTokens[0].SpacesBefore = 1
	}
	tokens = append(tokens, valueTokens...)

	if expr.Condition != nil {
		condTokens, d := g.exprTokens(expr.Condition, schema.AnyType)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte("if"), SpacesBefore: 1})
		if len(condTokens) > 0 {
			condTokens[0].SpacesBefore = 1
		}
		tokens = append(tokens, condTokens...)
	}

	if expr.Group {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenEllipsis, Bytes: []byte("..."), SpacesBefore: 0})
	}

	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenTemplateSeqEnd, Bytes: []byte{close}})
	return tokens, diags
}

func (g *generator) funcCallTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	switch expr.Name {
	case "__convert":
		// __convert is a PCL internal type-coercion function; it's an identity operation at runtime.
		if len(expr.Args) == 1 {
			return g.exprTokens(expr.Args[0], schema.AnyType)
		}
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
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
	case "unsecret":
		return g.passthroughFuncCallTokens("nonsensitive", expr.Args)
	case "singleOrNone":
		return g.passthroughFuncCallTokens("one", expr.Args)
	case "toBase64":
		return g.passthroughFuncCallTokens("base64encode", expr.Args)
	case "fromBase64":
		return g.passthroughFuncCallTokens("base64decode", expr.Args)
	case "toJSON":
		return g.passthroughFuncCallTokens("jsonencode", expr.Args)
	case "notImplemented":
		return g.notImplementedTokens(expr)
	default:
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}
}

// notImplementedTokens handles PCL's notImplemented("expression") by extracting the original
// expression text and emitting it as HCL when the expression uses a function that HCL supports.
// If the expression doesn't parse, isn't a function call, or uses an unknown function,
// it falls through to emit notImplemented(...) verbatim.
func (g *generator) notImplementedTokens(expr *model.FunctionCallExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	if len(expr.Args) != 1 {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	exprText, ok := extractStringLiteral(expr.Args[0])
	if !ok {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	parsed, diags := hclsyntax.ParseExpression([]byte(exprText), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	funcCall, ok := parsed.(*hclsyntax.FunctionCallExpr)
	if !ok {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	knownFunctions := eval.Functions("")
	if _, known := knownFunctions[funcCall.Name]; !known {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	syntaxTokens, diags := hclsyntax.LexExpression([]byte(exprText), "", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return g.passthroughFuncCallTokens(expr.Name, expr.Args)
	}

	var tokens hclwrite.Tokens
	for _, t := range syntaxTokens {
		if t.Type == hclsyntax.TokenEOF {
			continue
		}
		tokens = append(tokens, &hclwrite.Token{
			Type:  t.Type,
			Bytes: t.Bytes,
		})
	}
	return tokens, nil
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
	hclType, diags := packages.PulumiTokenToHCL(res.Token)
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

// schemaAwareRewriteTraversal rewrites a traversal's attribute names from PCL (camelCase)
// to HCL (snake_case), using schema property definitions to correctly map names through
// nested objects, maps, and arrays. When schema information is unavailable for a step,
// the remaining traversal is returned unchanged.
func schemaAwareRewriteTraversal(props []*schema.Property, traversal hcl.Traversal) hcl.Traversal {
	if len(traversal) == 0 || len(props) == 0 {
		return traversal
	}
	t, ok := traversal[0].(hcl.TraverseAttr)
	if !ok {
		return traversal
	}
	for _, p := range props {
		if p.Name == t.Name {
			t = hcl.TraverseAttr{Name: transform.SnakeCaseFromPulumiCase(p.Name), SrcRange: t.SrcRange}
			return append(hcl.Traversal{t}, schemaAwareRewriteTyped(p.Type, traversal[1:])...)
		}
	}
	return traversal
}

// schemaAwareRewriteTyped rewrites a traversal's attribute names using schema type
// information, dispatching to [schemaAwareRewriteTraversal] for object properties and
// recursing through map/array element types for index steps.
func schemaAwareRewriteTyped(typ schema.Type, traversal hcl.Traversal) hcl.Traversal {
	if len(traversal) == 0 {
		return traversal
	}

	switch t := traversal[0].(type) {
	case hcl.TraverseAttr:
		switch s := codegen.UnwrapType(typ).(type) {
		case *schema.ResourceType:
			return schemaAwareRewriteTraversal(s.Resource.Properties, traversal)
		case *schema.ObjectType:
			return schemaAwareRewriteTraversal(s.Properties, traversal)
		default:
			return traversal
		}
	case hcl.TraverseIndex:
		switch s := codegen.UnwrapType(typ).(type) {
		case *schema.MapType:
			return append(hcl.Traversal{t}, schemaAwareRewriteTyped(s.ElementType, traversal[1:])...)
		case *schema.ArrayType:
			return append(hcl.Traversal{t}, schemaAwareRewriteTyped(s.ElementType, traversal[1:])...)
		default:
			return traversal
		}
	default:
		return traversal
	}
}

// traversalStepsToTokens converts traversal steps (attrs and indexes) to HCL write tokens.
// Attribute names are emitted as-is — callers should rewrite names beforehand if needed.
func traversalStepsToTokens(traversal hcl.Traversal) hclwrite.Tokens {
	var tokens hclwrite.Tokens
	for _, step := range traversal {
		switch s := step.(type) {
		case hcl.TraverseAttr:
			tokens = append(tokens,
				&hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
				&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(s.Name)},
			)
		case hcl.TraverseIndex:
			keyTokens := hclwrite.TokensForValue(s.Key)
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")})
			tokens = append(tokens, keyTokens...)
			tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")})
		}
	}
	return tokens
}

// naiveRewriteTraversal rewrites all TraverseAttr names in a traversal from camelCase to
// snake_case without schema information. Used as a fallback when schema is unavailable.
func naiveRewriteTraversal(traversal hcl.Traversal) hcl.Traversal {
	rewritten := make(hcl.Traversal, len(traversal))
	for i, step := range traversal {
		if attr, ok := step.(hcl.TraverseAttr); ok {
			rewritten[i] = hcl.TraverseAttr{
				Name:     transform.SnakeCaseFromPulumiCase(attr.Name),
				SrcRange: attr.SrcRange,
			}
		} else {
			rewritten[i] = step
		}
	}
	return rewritten
}

// splatElementProps resolves the schema properties of the element type that a splat
// expression iterates over. It walks the Source expression's schema through its traversal
// to find the array type, then returns the element's properties.
func splatElementProps(source model.Expression) []*schema.Property {
	scope, ok := source.(*model.ScopeTraversalExpression)
	if !ok || len(scope.Parts) == 0 {
		return nil
	}
	res, ok := scope.Parts[0].(*pcl.Resource)
	if !ok || res.Schema == nil {
		return nil
	}
	// Walk through the source traversal to find the final schema type.
	// Start from the resource's properties, skip the root traversal step.
	var typ schema.Type = &schema.ObjectType{Properties: res.Schema.Properties}
	for _, step := range scope.Traversal[1:] {
		if typ == nil {
			return nil
		}
		typ = traverseSchemaType(typ, step)
	}
	// The source type should be an array; return the element's properties.
	if arr, ok := codegen.UnwrapType(typ).(*schema.ArrayType); ok {
		if obj, ok := codegen.UnwrapType(arr.ElementType).(*schema.ObjectType); ok {
			return obj.Properties
		}
	}
	return nil
}

// traverseSchemaType applies a single traversal step to a schema type, returning the
// resulting type. Returns nil if the step cannot be applied.
func traverseSchemaType(typ schema.Type, step hcl.Traverser) schema.Type {
	switch s := step.(type) {
	case hcl.TraverseAttr:
		switch t := codegen.UnwrapType(typ).(type) {
		case *schema.ResourceType:
			return findPropertyType(t.Resource.Properties, s.Name)
		case *schema.ObjectType:
			return findPropertyType(t.Properties, s.Name)
		}
	case hcl.TraverseIndex:
		switch t := codegen.UnwrapType(typ).(type) {
		case *schema.MapType:
			return t.ElementType
		case *schema.ArrayType:
			return t.ElementType
		}
	}
	return nil
}

func findPropertyType(props []*schema.Property, name string) schema.Type {
	for _, p := range props {
		if p.Name == name {
			return p.Type
		}
	}
	return nil
}

// traverseNameStep returns a TraverseAttr for valid HCL identifiers (e.g. `.foo`),
// or a TraverseIndex with a string key for names that contain special characters
// (e.g. `["aA-Alpha_alpha.🤯⁉️"]`).
func traverseNameStep(name string) hcl.Traverser {
	if hclsyntax.ValidIdentifier(name) {
		return hcl.TraverseAttr{Name: name}
	}
	return hcl.TraverseIndex{Key: cty.StringVal(name)}
}

// scopeTraversalTokens generates HCL tokens for a scope traversal expression.
// PCL config variables become HCL `var.<name>`, local variables become `local.<name>`,
// and resource references become `<resource_type>.<name>.<property>`.
func (g *generator) scopeTraversalTokens(expr *model.ScopeTraversalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	if len(expr.Parts) == 0 {
		return hclwrite.TokensForTraversal(expr.Traversal), nil
	}

	traversal := expr.Traversal
	switch part := expr.Parts[0].(type) {
	case *pcl.ConfigVariable:
		// Rewrite "aMap.x" → "var.aMap.x".
		rewritten := make(hcl.Traversal, 0, len(traversal)+1)
		rewritten = append(rewritten, hcl.TraverseRoot{Name: "var"})
		rewritten = append(rewritten, traverseNameStep(part.LogicalName()))
		return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
	case *pcl.LocalVariable:
		// If this local is backed by an invoke call, substitute the data source reference
		// directly so that resource dependency tracking works correctly.
		if call, ok := part.Definition.Value.(*model.FunctionCallExpression); ok && call.Name == pcl.Invoke {
			for _, ds := range g.invokeDataSources {
				if ds.expr == call {
					token, ok := extractStringLiteral(call.Args[0])
					if ok {
						dsType, d := packages.PulumiTokenToHCL(token)
						if !d.HasErrors() {
							rewritten := hcl.Traversal{
								hcl.TraverseRoot{Name: "data"},
								hcl.TraverseAttr{Name: dsType},
								hcl.TraverseAttr{Name: ds.name},
							}
							return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
						}
					}
					break
				}
			}
		}
		// Rewrite "myLocal.x" → "local.myLocal.x".
		rewritten := make(hcl.Traversal, 0, len(traversal)+1)
		rewritten = append(rewritten, hcl.TraverseRoot{Name: "local"})
		rewritten = append(rewritten, traverseNameStep(part.LogicalName()))
		return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
	case *pcl.Resource:
		// Rewrite "myResource.property" → "resource_type.myResource.property".
		//
		// TODO: Resource traversal needs to be type (and schema) aware. It needs to invoke
		// [transform.SnakeCaseFromPulumiCase] on property values, and the invoke the standard ["<key>"]
		// & [<idx>] operators otherwise.
		hclType, diags := packages.PulumiTokenToHCL(part.Token)
		if diags.HasErrors() {
			return nil, diags
		}
		rewritten := make(hcl.Traversal, 0, len(traversal)+1)
		rewritten = append(rewritten, hcl.TraverseRoot{Name: hclType})
		rewritten = append(rewritten, traverseNameStep(part.LogicalName()))
		return hclwrite.TokensForTraversal(append(rewritten, schemaAwareRewriteTraversal(part.Schema.Properties, traversal[1:])...)), nil
	case *pcl.Component:
		// Rewrite "someComponent.output" → "module.someComponent.output".
		rewritten := make(hcl.Traversal, 0, len(traversal)+1)
		rewritten = append(rewritten, hcl.TraverseRoot{Name: "module"})
		rewritten = append(rewritten, traverseNameStep(part.LogicalName()))
		return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
	default:
		if traversal.RootName() == "range" {
			switch g.currentRangeKind {
			case rangeKindCount:
				return hclwrite.TokensForTraversal(hcl.Traversal{
					hcl.TraverseRoot{Name: "count"},
					hcl.TraverseAttr{Name: "index"},
				}), nil
			default: // rangeKindForEach
				return hclwrite.TokensForTraversal(append(
					hcl.Traversal{hcl.TraverseRoot{Name: "each"}}, traversal[1:]...,
				)), nil
			}
		}
		if db := g.dynamicBlock; db != nil {
			// Rewrite references to the for-expression's iterator variables:
			//   valueVar.field → blockName.value.field
			//   keyVar         → blockName.key
			if db.valueVariable != nil && part == db.valueVariable {
				rewritten := hcl.Traversal{
					hcl.TraverseRoot{Name: db.blockName},
					hcl.TraverseAttr{Name: "value"},
				}
				return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
			}
			if db.keyVariable != nil && part == db.keyVariable {
				rewritten := hcl.Traversal{
					hcl.TraverseRoot{Name: db.blockName},
					hcl.TraverseAttr{Name: "key"},
				}
				return hclwrite.TokensForTraversal(append(rewritten, traversal[1:]...)), nil
			}
		}
		return hclwrite.TokensForTraversal(traversal), nil
	}
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
		argTokens, d := g.exprTokens(arg, schema.AnyType)
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
	collTokens, diags := g.exprTokens(expr.Collection, schema.AnyType)
	if diags.HasErrors() {
		return nil, diags
	}
	keyTokens, d := g.exprTokens(expr.Key, schema.AnyType)
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
	sourceTokens, diags := g.exprTokens(expr.Source, schema.AnyType)
	if diags.HasErrors() {
		return nil, diags
	}
	return append(sourceTokens, traversalStepsToTokens(naiveRewriteTraversal(expr.Traversal))...), diags
}

// splatTokens generates HCL tokens for a PCL SplatExpression.
//
// PCL:  source.details[*].value
// HCL:  source.details[*].value
//
// The PCL binder merges the relative traversal after [*] into the
// ScopeTraversalExpression rooted at the SplatVariable. So Each is typically a
// ScopeTraversalExpression with Parts[0]=SplatVariable and traversal steps
// [TraverseRoot, TraverseAttr("value"), ...]. We emit source, [*], then the
// traversal steps after the root.
func (g *generator) splatTokens(expr *model.SplatExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	sourceTokens, diags := g.exprTokens(expr.Source, schema.AnyType)
	if diags.HasErrors() {
		return nil, diags
	}

	tokens := append(sourceTokens,
		&hclwrite.Token{Type: hclsyntax.TokenOBrack, Bytes: []byte("[")},
		&hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")},
		&hclwrite.Token{Type: hclsyntax.TokenCBrack, Bytes: []byte("]")},
	)

	// Extract the traversal from the Each expression, skipping the root
	// (which is the SplatVariable, already represented by [*]).
	var eachTraversal hcl.Traversal
	switch each := expr.Each.(type) {
	case *model.ScopeTraversalExpression:
		eachTraversal = each.Traversal[1:]
	case *model.RelativeTraversalExpression:
		eachTraversal = each.Traversal
	default:
		return nil, hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported splat each expression",
			Detail:   fmt.Sprintf("splat each expression type %T is not yet supported", expr.Each),
		}}
	}

	// Rewrite attribute names using schema info from the source's element type,
	// falling back to naive camelCase → snake_case conversion.
	if props := splatElementProps(expr.Source); len(props) > 0 {
		eachTraversal = schemaAwareRewriteTraversal(props, eachTraversal)
	} else {
		eachTraversal = naiveRewriteTraversal(eachTraversal)
	}

	tokens = append(tokens, traversalStepsToTokens(eachTraversal)...)
	return tokens, diags
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
		elemTokens, d := g.exprTokens(elem, schema.AnyType)
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

func (g *generator) objectTokens(expr *model.ObjectConsExpression, typ schema.Type) (hclwrite.Tokens, hcl.Diagnostics) {
	tokens := hclwrite.Tokens{
		{Type: hclsyntax.TokenOBrace, Bytes: []byte("{")},
	}
	if len(expr.Items) == 0 {
		tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCBrace, Bytes: []byte("}")})
		return tokens, nil
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenNewline, Bytes: []byte("\n")})

	keyName := func(key model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
		return g.exprTokens(key, schema.StringType)
	}
	propType := func(key model.Expression) schema.Type {
		return schema.AnyType
	}
	switch typ := codegen.UnwrapType(typ).(type) {
	case *schema.ObjectType:
		keyName = func(key model.Expression) (hclwrite.Tokens, hcl.Diagnostics) {
			name, _ := extractStringLiteral(key)
			return hclwrite.TokensForIdentifier(transform.SnakeCaseFromPulumiCase(name)), nil
		}
		propType = func(key model.Expression) schema.Type {
			name, _ := extractStringLiteral(key)
			if p, ok := typ.Property(name); ok {
				return p.Type
			}
			return schema.AnyType
		}
	case *schema.MapType:
		propType = func(model.Expression) schema.Type {
			return typ.ElementType
		}
	}

	var diags hcl.Diagnostics
	for _, item := range expr.Items {
		keyTokens, d := keyName(item.Key)
		diags = append(diags, d...)
		if d.HasErrors() {
			return nil, diags
		}
		valTokens, d := g.exprTokens(item.Value, propType(item.Key))
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
			partTokens, d := g.exprTokens(part, schema.AnyType)
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
	leftTokens, diags := g.exprTokens(expr.LeftOperand, schema.AnyType)
	if diags.HasErrors() {
		return nil, diags
	}

	rightTokens, d := g.exprTokens(expr.RightOperand, schema.AnyType)
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

// conditionalTokens generates HCL tokens for a conditional expression (condition ? true : false).
func (g *generator) conditionalTokens(expr *model.ConditionalExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	condTokens, diags := g.exprTokens(expr.Condition, schema.AnyType)
	if diags.HasErrors() {
		return nil, diags
	}

	trueTokens, d := g.exprTokens(expr.TrueResult, schema.AnyType)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	falseTokens, d := g.exprTokens(expr.FalseResult, schema.AnyType)
	diags = append(diags, d...)
	if d.HasErrors() {
		return nil, diags
	}

	tokens := condTokens
	tokens = append(tokens, &hclwrite.Token{
		Type: hclsyntax.TokenQuestion, Bytes: []byte("?"), SpacesBefore: 1,
	})
	if len(trueTokens) > 0 {
		trueTokens[0].SpacesBefore = 1
	}
	tokens = append(tokens, trueTokens...)
	tokens = append(tokens, &hclwrite.Token{
		Type: hclsyntax.TokenColon, Bytes: []byte(":"), SpacesBefore: 1,
	})
	if len(falseTokens) > 0 {
		falseTokens[0].SpacesBefore = 1
	}
	tokens = append(tokens, falseTokens...)
	return tokens, diags
}

// unaryOpTokens generates HCL tokens for a unary operation expression.
func (g *generator) unaryOpTokens(expr *model.UnaryOpExpression) (hclwrite.Tokens, hcl.Diagnostics) {
	operandTokens, diags := g.exprTokens(expr.Operand, schema.AnyType)
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
