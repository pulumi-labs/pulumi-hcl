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

// Package converter converts HCL programs to PCL (Pulumi Configuration Language).
package converter

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"github.com/pulumi/pulumi/sdk/v3/go/common/workspace"
	"gopkg.in/yaml.v3"
)

type hclConverter struct{}

// New returns a new HCL->PCL converter.
func New() plugin.Converter { return &hclConverter{} }

func (*hclConverter) Close() error { return nil }

func (*hclConverter) ConvertState(
	_ context.Context, _ *plugin.ConvertStateRequest,
) (*plugin.ConvertStateResponse, error) {
	return nil, errors.New("not implemented")
}

func (*hclConverter) ConvertProgram(
	ctx context.Context, req *plugin.ConvertProgramRequest,
) (*plugin.ConvertProgramResponse, error) {
	if req.LoaderTarget == "" {
		return nil, fmt.Errorf("missing loader address")
	}

	client, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("creating loader client: %w", err)
	}
	defer contract.IgnoreClose(client)

	paramInfos, err := readParameterizationInfos(req.SourceDirectory)
	if err != nil {
		return nil, fmt.Errorf("reading parameterization infos: %w", err)
	}

	loader := schema.ReferenceLoader(schema.NewCachedLoader(client))
	if len(paramInfos) > 0 {
		loader = packages.NewParameterizationAwareLoader(loader, paramInfos)
	}

	// Read source Pulumi.yaml, extract project name, write target Pulumi.yaml with runtime: pcl.
	pulumiYAMLBytes, err := os.ReadFile(filepath.Join(req.SourceDirectory, "Pulumi.yaml"))
	if err != nil {
		return nil, fmt.Errorf("reading Pulumi.yaml: %w", err)
	}

	var project map[string]any
	if err := yaml.Unmarshal(pulumiYAMLBytes, &project); err != nil {
		return nil, fmt.Errorf("parsing Pulumi.yaml: %w", err)
	}
	project["runtime"] = "pcl"

	targetYAML, err := yaml.Marshal(project)
	if err != nil {
		return nil, fmt.Errorf("marshaling Pulumi.yaml: %w", err)
	}

	if err := os.MkdirAll(req.TargetDirectory, 0o755); err != nil {
		return nil, fmt.Errorf("creating target directory: %w", err)
	}

	if err := os.WriteFile(filepath.Join(req.TargetDirectory, "Pulumi.yaml"), targetYAML, 0o644); err != nil {
		return nil, fmt.Errorf("writing Pulumi.yaml: %w", err)
	}

	// Walk source dir tree for .hcl files, transform each to a .pp PCL file.
	var allDiags hcl.Diagnostics

	err = filepath.WalkDir(req.SourceDirectory, func(path string, d os.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			if strings.HasPrefix(d.Name(), ".") {
				return filepath.SkipDir
			}
			return nil
		}
		if !strings.HasSuffix(d.Name(), ".hcl") {
			return nil
		}

		relPath, err := filepath.Rel(req.SourceDirectory, path)
		if err != nil {
			return fmt.Errorf("computing relative path for %s: %w", path, err)
		}

		content, err := os.ReadFile(path)
		if err != nil {
			return fmt.Errorf("reading %s: %w", relPath, err)
		}

		out := hclwrite.NewEmptyFile()
		// Only emit package blocks for root-level files.
		var pi map[string]workspace.PackageDescriptor
		if filepath.Dir(relPath) == "." {
			pi = paramInfos
		}
		diags, err := transformHCLFileToPCL(ctx, content, d.Name(), out.Body(), loader, pi)
		if err != nil {
			return err
		}
		allDiags = append(allDiags, diags...)

		dstRelPath := strings.TrimSuffix(relPath, ".hcl") + ".pp"
		dstPath := filepath.Join(req.TargetDirectory, dstRelPath)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0o755); err != nil {
			return fmt.Errorf("creating directory for %s: %w", dstRelPath, err)
		}

		f, err := os.Create(dstPath)
		if err != nil {
			return fmt.Errorf("opening %s: %w", dstRelPath, err)
		}
		defer contract.IgnoreClose(f)
		if _, err := out.WriteTo(f); err != nil {
			return fmt.Errorf("writing to %s: %w", dstRelPath, err)
		}
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking source directory: %w", err)
	}

	return &plugin.ConvertProgramResponse{Diagnostics: allDiags}, nil
}

// callReference identifies a call block by resource name and method name.
type callReference struct {
	resourceName string
	methodName   string
}

// dataReference identifies a data block by HCL type and logical name.
type dataReference struct {
	hclType string
	name    string
}

// fileTransformer holds context for converting a single HCL file to PCL.
type fileTransformer struct {
	src             []byte
	knownHCLTypes   map[string]bool               // set of HCL type labels used in resource blocks
	stackRefNames   map[string]bool               // set of logical names of pulumi_stackreference resources
	knownProviders  []string                      // provider names from terraform.required_providers
	callBlocks      map[callReference]*hclsyntax.Body
	dataBlocks      map[dataReference]*hclsyntax.Body
	dataTokens      map[string]string             // key: hclType, value: resolved PCL token
	loader          schema.ReferenceLoader
	resourceSchemas map[string]*schema.Resource   // cache: HCL type label → resolved schema resource
}

// newFileTransformer creates a fileTransformer by pre-scanning body for resource and data definitions.
func newFileTransformer(ctx context.Context, src []byte, body *hclsyntax.Body, loader schema.ReferenceLoader) (*fileTransformer, hcl.Diagnostics) {
	ft := &fileTransformer{
		src:             src,
		knownHCLTypes:   make(map[string]bool),
		stackRefNames:   make(map[string]bool),
		callBlocks:      make(map[callReference]*hclsyntax.Body),
		dataBlocks:      make(map[dataReference]*hclsyntax.Body),
		dataTokens:      make(map[string]string),
		loader:          loader,
		resourceSchemas: make(map[string]*schema.Resource),
	}
	var diags hcl.Diagnostics
	for _, block := range body.Blocks {
		if block.Type == "terraform" {
			for _, sub := range block.Body.Blocks {
				if sub.Type == "required_providers" {
					for name := range sub.Body.Attributes {
						ft.knownProviders = append(ft.knownProviders, name)
					}
				}
			}
		}
		if block.Type == "resource" && len(block.Labels) >= 1 {
			ft.knownHCLTypes[block.Labels[0]] = true
			if block.Labels[0] == "pulumi_stackreference" && len(block.Labels) >= 2 {
				ft.stackRefNames[block.Labels[1]] = true
			}
		}
		if block.Type == "call" && len(block.Labels) == 2 {
			ft.callBlocks[callReference{block.Labels[0], block.Labels[1]}] = block.Body
		}
		if block.Type == "data" && len(block.Labels) == 2 {
			hclType := block.Labels[0]
			name := block.Labels[1]
			ft.dataBlocks[dataReference{hclType, name}] = block.Body
			if _, seen := ft.dataTokens[hclType]; !seen {
				fn, err := packages.ResolveFunction(ctx, loader, ft.knownProviders, hclType)
				if err != nil {
					diags = append(diags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "unknown data source type",
						Detail:   fmt.Sprintf("cannot convert HCL type %q to PCL token: %v", hclType, err),
						Subject:  block.TypeRange.Ptr(),
					})
					ft.dataTokens[hclType] = ""
				} else {
					ft.dataTokens[hclType] = fn.Token
				}
			}
		}
	}
	return ft, diags
}

// resolveHCLType resolves an HCL resource type label (e.g., "pulumi_stash") to a schema
// resource, caching the result for subsequent calls.
func (ft *fileTransformer) resolveHCLType(ctx context.Context, hclType string) (*schema.Resource, error) {
	if res, ok := ft.resourceSchemas[hclType]; ok {
		return res, nil
	}
	res, err := packages.ResolveResource(ctx, ft.loader, ft.knownProviders, hclType)
	if err != nil {
		return nil, fmt.Errorf("resolving resource type %q: %w", hclType, err)
	}
	ft.resourceSchemas[hclType] = res
	return res, nil
}

// invokeOptionHCLToPCL maps HCL data-block attribute names that represent invoke
// options (written by genInvokeOptions) to their PCL invoke-options object keys.
var invokeOptionHCLToPCL = map[string]string{
	"depends_on":          "dependsOn",
	"parent":              "parent",
	"plugin_download_url": "pluginDownloadUrl",
	"provider":            "provider",
	"version":             "version",
}

// resourceOptionHCLToPCL maps HCL attribute names (written by genResourceOptions)
// to the corresponding PCL options-block attribute names (camelCase).
var resourceOptionHCLToPCL = map[string]string{
	"aliases":                   "aliases",
	"additional_secret_outputs": "additionalSecretOutputs",
	"deleted_with":              "deletedWith",
	"depends_on":                "dependsOn",
	"env_var_mappings":          "envVarMappings",
	"hide_diffs":                "hideDiffs",
	"import_id":                 "import",
	"parent":                    "parent",
	"plugin_download_url":       "pluginDownloadURL",
	"provider":                  "provider",
	"providers":                 "providers",
	"range":                     "range",
	"replace_on_changes":        "replaceOnChanges",
	"replace_with":              "replaceWith",
	"replacement_trigger":       "replacementTrigger",
	"retain_on_delete":          "retainOnDelete",
	"version":                   "version",
}

// transformHCLFileToPCL converts HCL source bytes to PCL source bytes.
// It returns any non-fatal diagnostics (e.g., unsupported block types).
func transformHCLFileToPCL(
	ctx context.Context,
	src []byte, filename string, out *hclwrite.Body,
	loader schema.ReferenceLoader,
	paramInfos map[string]workspace.PackageDescriptor,
) (hcl.Diagnostics, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return diags, nil
	}

	body := file.Body.(*hclsyntax.Body)
	ft, resultDiags := newFileTransformer(ctx, src, body, loader)
	if resultDiags.HasErrors() {
		return resultDiags, nil
	}

	for _, alias := range sortedKeys(paramInfos) {
		emitPackageBlock(out, alias, paramInfos[alias])
	}

	for _, block := range body.Blocks {
		switch block.Type {

		case "terraform":
			// Skip: no PCL equivalent.

		case "call":
			// Call blocks are inlined into expressions as call() function calls; skip block output.

		case "data":
			// Data blocks are inlined into expressions as invoke() calls; skip block output.

		case "variable":
			if len(block.Labels) == 0 {
				continue
			}
			labels := []string{block.Labels[0] /* name */}

			if typeAttr, ok := block.Body.Attributes["type"]; ok {
				labels = append(labels, convertHCLTypeExpr(src, typeAttr.Expr))
			}
			out.AppendNewBlock("config", labels)
			out.AppendNewline()

		case "locals":
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				out.SetAttributeRaw(attr.Name, ft.transformExpr(attr.Expr))
				out.AppendNewline()
			}

		case "output":
			if len(block.Labels) == 0 {
				continue
			}
			name := block.Labels[0]
			blk := out.AppendNewBlock("output", []string{name})
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				blk.Body().SetAttributeRaw(attr.Name, ft.transformExpr(attr.Expr))
			}
			out.AppendNewline()

		case "module":
			if len(block.Labels) == 0 {
				continue
			}
			logicalName := block.Labels[0]
			sourceAttr, ok := block.Body.Attributes["source"]
			if !ok {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "missing source attribute",
					Detail:   "module block requires a \"source\" attribute",
					Subject:  block.TypeRange.Ptr(),
				})
				continue
			}
			sourceVal, valDiags := sourceAttr.Expr.Value(nil)
			if valDiags.HasErrors() {
				resultDiags = append(resultDiags, valDiags...)
				continue
			}
			if sourceVal.Type() != cty.String {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "invalid source attribute",
					Detail:   "module \"source\" must be a string",
					Subject:  sourceAttr.Expr.Range().Ptr(),
				})
				continue
			}
			blk := out.AppendNewBlock("component", []string{logicalName, sourceVal.AsString()})
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				if attr.Name == "source" {
					continue
				}
				blk.Body().SetAttributeRaw(attr.Name, ft.transformExpr(attr.Expr))
			}
			out.AppendNewline()

		case "resource":
			if len(block.Labels) < 2 {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "malformed resource block",
					Detail:   "resource block requires exactly 2 labels: <type> and <name>",
					Subject:  block.TypeRange.Ptr(),
				})
				continue
			}
			hclType := block.Labels[0]
			logicalName := block.Labels[1]

			res, err := ft.resolveHCLType(ctx, hclType)
			if err != nil {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "unknown resource type",
					Detail:   fmt.Sprintf("cannot convert HCL type %q to PCL token: %v", hclType, err),
					Subject:  block.TypeRange.Ptr(),
				})
				continue
			}

			blk := out.AppendNewBlock("resource", []string{logicalName, res.Token})

			// Emit input properties first (skip resource option attributes).
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				if _, isOpt := resourceOptionHCLToPCL[attr.Name]; isOpt {
					continue
				}
				name, _ := transform.PulumiCaseFromSnakeCase(attr.Name, res.InputProperties)
				blk.Body().SetAttributeRaw(name, ft.transformExpr(attr.Expr))
			}

			// Collect resource options from attributes and sub-blocks.
			type optEntry struct {
				name   string
				tokens hclwrite.Tokens
			}
			var opts []optEntry
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				if pclName, isOpt := resourceOptionHCLToPCL[attr.Name]; isOpt {
					var tokens hclwrite.Tokens
					switch attr.Name {
					case "hide_diffs", "replace_on_changes":
						tokens = ft.transformPropertyPathList(attr.Expr)
					default:
						tokens = ft.transformExpr(attr.Expr)
					}
					opts = append(opts, optEntry{pclName, tokens})
				}
			}
			for _, subBlock := range block.Body.Blocks {
				switch subBlock.Type {
				case "lifecycle":
					for _, attr := range sortedAttributes(subBlock.Body.Attributes) {
						switch attr.Name {
						case "prevent_destroy":
							opts = append(opts, optEntry{"protect", ft.transformExpr(attr.Expr)})
						case "ignore_changes":
							opts = append(opts, optEntry{"ignoreChanges", ft.transformPropertyPathList(attr.Expr)})
						case "create_before_destroy":
							// The codegen writes create_before_destroy = !deleteBeforeReplace.
							// Invert to recover deleteBeforeReplace.
							opts = append(opts, optEntry{"deleteBeforeReplace", invertTokens(ft.transformExpr(attr.Expr))})
						default:
							resultDiags = append(resultDiags, &hcl.Diagnostic{
								Severity: hcl.DiagError,
								Summary:  "unsupported lifecycle attribute",
								Detail:   fmt.Sprintf("lifecycle attribute %q is not supported by the HCL converter", attr.Name),
								Subject:  attr.NameRange.Ptr(),
							})
						}
					}
				case "timeouts":
					var timeoutAttrs []hclwrite.ObjectAttrTokens
					for _, attr := range sortedAttributes(subBlock.Body.Attributes) {
						timeoutAttrs = append(timeoutAttrs, hclwrite.ObjectAttrTokens{
							Name:  hclwrite.TokensForIdentifier(attr.Name),
							Value: ft.transformExpr(attr.Expr),
						})
					}
					if len(timeoutAttrs) > 0 {
						opts = append(opts, optEntry{"customTimeouts", hclwrite.TokensForObject(timeoutAttrs)})
					}
				default:
					resultDiags = append(resultDiags, &hcl.Diagnostic{
						Severity: hcl.DiagError,
						Summary:  "unsupported resource sub-block",
						Detail:   fmt.Sprintf("resource sub-block %q is not supported by the HCL converter", subBlock.Type),
						Subject:  subBlock.TypeRange.Ptr(),
					})
				}
			}
			if len(opts) > 0 {
				optBlk := blk.Body().AppendNewBlock("options", nil)
				for _, o := range opts {
					optBlk.Body().SetAttributeRaw(o.name, o.tokens)
				}
			}
			out.AppendNewline()

		case "pulumi":
			blk := out.AppendNewBlock("pulumi", nil)
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				name, _ := transform.PulumiCaseFromSnakeCase(attr.Name, nil)
				blk.Body().SetAttributeRaw(name, ft.transformExpr(attr.Expr))
			}
			out.AppendNewline()

		default:
			resultDiags = append(resultDiags, &hcl.Diagnostic{
				Severity: hcl.DiagError,
				Summary:  "unsupported block type",
				Detail:   fmt.Sprintf("block type %q is not supported by the HCL converter", block.Type),
				Subject:  block.TypeRange.Ptr(),
			})
		}
	}

	// Top-level attributes (uncommon in HCL input, but pass through with transforms).
	for _, attr := range sortedAttributes(body.Attributes) {
		out.SetAttributeRaw(attr.Name, ft.transformExpr(attr.Expr))
		out.AppendNewline()
	}

	return resultDiags, nil
}

// sortedAttributes returns attributes sorted by source position.
func sortedAttributes(attrs hclsyntax.Attributes) []*hclsyntax.Attribute {
	result := slices.Collect(maps.Values(attrs))
	sort.Slice(result, func(i, j int) bool {
		return result[i].NameRange.Start.Byte < result[j].NameRange.Start.Byte
	})
	return result
}

// convertHCLTypeExpr converts an HCL type constraint expression to a PCL type string.
func convertHCLTypeExpr(src []byte, expr hclsyntax.Expression) string {
	return convertHCLTypeExprInner(src, expr, false)
}

// convertHCLTypeExprInner converts an HCL type expression to a PCL type string.
// inCollection is true when the expression is a type argument inside a collection
// type (map, list, set, object), where HCL's "number" maps to PCL's "int".
func convertHCLTypeExprInner(src []byte, expr hclsyntax.Expression, inCollection bool) string {
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		switch e.Traversal.RootName() {
		case "string":
			return "string"
		case "bool":
			return "bool"
		case "number":
			if inCollection {
				return "int"
			}
			return "number"
		case "any":
			return "any"
		}
	case *hclsyntax.FunctionCallExpr:
		args := make([]string, len(e.Args))
		for i, arg := range e.Args {
			args[i] = convertHCLTypeExprInner(src, arg, true)
		}
		return e.Name + "(" + strings.Join(args, ", ") + ")"
	case *hclsyntax.ObjectConsExpr:
		parts := make([]string, len(e.Items))
		for i, item := range e.Items {
			key := convertHCLTypeExprInner(src, item.KeyExpr, false)
			val := convertHCLTypeExprInner(src, item.ValueExpr, true)
			parts[i] = key + "=" + val
		}
		return "{" + strings.Join(parts, ", ") + "}"
	}
	// Fallback: use source bytes.
	return string(src[expr.Range().Start.Byte:expr.Range().End.Byte])
}

// transformExpr converts an HCL expression to PCL hclwrite tokens by walking the AST.
func (ft *fileTransformer) transformExpr(expr hclsyntax.Expression) hclwrite.Tokens {
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		return ft.transformTraversal(e)
	case *hclsyntax.FunctionCallExpr:
		args := make([]hclwrite.Tokens, len(e.Args))
		for i, arg := range e.Args {
			args[i] = ft.transformExpr(arg)
		}
		return hclwrite.TokensForFunctionCall(transformFunctionName(e.Name), args...)
	case *hclsyntax.TemplateExpr:
		return ft.transformTemplate(e)
	case *hclsyntax.TemplateWrapExpr:
		return ft.transformExpr(e.Wrapped)
	case *hclsyntax.TupleConsExpr:
		var elems []hclwrite.Tokens
		for _, item := range e.Exprs {
			elems = append(elems, ft.transformExpr(item))
		}
		return hclwrite.TokensForTuple(elems)
	case *hclsyntax.ObjectConsKeyExpr:
		// Identifier keys (e.g., bool_array) are property names: convert snake_case → camelCase.
		// Quoted string keys (e.g., "my key") are map keys: pass through the wrapped expression.
		if name := hcl.ExprAsKeyword(e); name != "" {
			camel, _ := transform.PulumiCaseFromSnakeCase(name, nil)
			return hclwrite.TokensForIdentifier(camel)
		}
		return ft.transformExpr(e.Wrapped)
	case *hclsyntax.ObjectConsExpr:
		var attrs []hclwrite.ObjectAttrTokens
		for _, item := range e.Items {
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  ft.transformExpr(item.KeyExpr),
				Value: ft.transformExpr(item.ValueExpr),
			})
		}
		return hclwrite.TokensForObject(attrs)
	case *hclsyntax.BinaryOpExpr:
		lhs := ft.transformExpr(e.LHS)
		op := binaryOpToken(e.Op)
		op.SpacesBefore = 1
		rhs := ft.transformExpr(e.RHS)
		if len(rhs) > 0 {
			rhs[0].SpacesBefore = 1
		}
		return append(append(lhs, op), rhs...)
	case *hclsyntax.UnaryOpExpr:
		op := unaryOpToken(e.Op)
		val := ft.transformExpr(e.Val)
		if len(val) > 0 {
			val[0].SpacesBefore = 1
		}
		return append(hclwrite.Tokens{op}, val...)
	default:
		r := expr.Range()
		return hclwrite.Tokens{
			&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: ft.src[r.Start.Byte:r.End.Byte]},
		}
	}
}

func (ft *fileTransformer) transformTemplate(e *hclsyntax.TemplateExpr) hclwrite.Tokens {
	tokens := hclwrite.Tokens{
		&hclwrite.Token{Type: hclsyntax.TokenOQuote, Bytes: []byte{'"'}},
	}
	for _, part := range e.Parts {
		if lit, ok := part.(*hclsyntax.LiteralValueExpr); ok {
			r := lit.SrcRange
			tokens = append(tokens, &hclwrite.Token{
				Type:  hclsyntax.TokenQuotedLit,
				Bytes: ft.src[r.Start.Byte:r.End.Byte],
			})
		} else {
			tokens = append(tokens,
				&hclwrite.Token{Type: hclsyntax.TokenTemplateInterp, Bytes: []byte("${")},
			)
			tokens = append(tokens, ft.transformExpr(part)...)
			tokens = append(tokens,
				&hclwrite.Token{Type: hclsyntax.TokenTemplateSeqEnd, Bytes: []byte("}")},
			)
		}
	}
	tokens = append(tokens, &hclwrite.Token{Type: hclsyntax.TokenCQuote, Bytes: []byte{'"'}})
	return tokens
}

func binaryOpToken(op *hclsyntax.Operation) *hclwrite.Token {
	switch op {
	case hclsyntax.OpAdd:
		return &hclwrite.Token{Type: hclsyntax.TokenPlus, Bytes: []byte("+")}
	case hclsyntax.OpSubtract:
		return &hclwrite.Token{Type: hclsyntax.TokenMinus, Bytes: []byte("-")}
	case hclsyntax.OpMultiply:
		return &hclwrite.Token{Type: hclsyntax.TokenStar, Bytes: []byte("*")}
	case hclsyntax.OpDivide:
		return &hclwrite.Token{Type: hclsyntax.TokenSlash, Bytes: []byte("/")}
	case hclsyntax.OpModulo:
		return &hclwrite.Token{Type: hclsyntax.TokenPercent, Bytes: []byte("%")}
	case hclsyntax.OpLogicalAnd:
		return &hclwrite.Token{Type: hclsyntax.TokenAnd, Bytes: []byte("&&")}
	case hclsyntax.OpLogicalOr:
		return &hclwrite.Token{Type: hclsyntax.TokenOr, Bytes: []byte("||")}
	case hclsyntax.OpEqual:
		return &hclwrite.Token{Type: hclsyntax.TokenEqualOp, Bytes: []byte("==")}
	case hclsyntax.OpNotEqual:
		return &hclwrite.Token{Type: hclsyntax.TokenNotEqual, Bytes: []byte("!=")}
	case hclsyntax.OpGreaterThan:
		return &hclwrite.Token{Type: hclsyntax.TokenGreaterThan, Bytes: []byte(">")}
	case hclsyntax.OpGreaterThanOrEqual:
		return &hclwrite.Token{Type: hclsyntax.TokenGreaterThanEq, Bytes: []byte(">=")}
	case hclsyntax.OpLessThan:
		return &hclwrite.Token{Type: hclsyntax.TokenLessThan, Bytes: []byte("<")}
	case hclsyntax.OpLessThanOrEqual:
		return &hclwrite.Token{Type: hclsyntax.TokenLessThanEq, Bytes: []byte("<=")}
	default:
		panic(fmt.Sprintf("unsupported binary operation: %v", op))
	}
}

func unaryOpToken(op *hclsyntax.Operation) *hclwrite.Token {
	switch op {
	case hclsyntax.OpNegate:
		return &hclwrite.Token{Type: hclsyntax.TokenMinus, Bytes: []byte("-")}
	case hclsyntax.OpLogicalNot:
		return &hclwrite.Token{Type: hclsyntax.TokenBang, Bytes: []byte("!")}
	default:
		panic(fmt.Sprintf("unsupported unary operation: %v", op))
	}
}

// transformTraversal converts an HCL scope traversal to PCL tokens.
func (ft *fileTransformer) transformTraversal(e *hclsyntax.ScopeTraversalExpr) hclwrite.Tokens {
	root := e.Traversal.RootName()

	// Resource traversal: strip the HCL type prefix (e.g., "pulumi_stash.myRes.prop" → "myRes.prop"),
	// and convert property attribute names from snake_case to camelCase.
	if ft.knownHCLTypes[root] {
		stripped := stripRoot(e.Traversal)
		// StackReference: <type>.<name>.outputs["key"] → getOutput(<name>, "key")
		if len(stripped) == 3 {
			logicalName, ok1 := stripped[0].(hcl.TraverseRoot)
			attr, ok2 := stripped[1].(hcl.TraverseAttr)
			idx, ok3 := stripped[2].(hcl.TraverseIndex)
			if ok1 && ok2 && ok3 && attr.Name == "outputs" &&
				ft.stackRefNames[logicalName.Name] && idx.Key.Type() == cty.String {
				refTokens := hclwrite.TokensForTraversal(hcl.Traversal{hcl.TraverseRoot{Name: logicalName.Name}})
				keyTokens := hclwrite.TokensForValue(idx.Key)
				return hclwrite.TokensForFunctionCall("getOutput", refTokens, keyTokens)
			}
		}
		return hclwrite.TokensForTraversal(ft.camelCaseTraversalAttrs(stripped, root))
	}

	switch root {
	case "data":
		// data.hclType.name[.prop...] → invoke("token", {args...})[.prop...]
		if len(e.Traversal) >= 3 {
			typeAttr, ok1 := e.Traversal[1].(hcl.TraverseAttr)
			nameAttr, ok2 := e.Traversal[2].(hcl.TraverseAttr)
			if ok1 && ok2 {
				tokens := ft.invokeExprTokens(typeAttr.Name, nameAttr.Name)
				for _, step := range e.Traversal[3:] {
					if attr, ok := step.(hcl.TraverseAttr); ok {
						camel, _ := transform.PulumiCaseFromSnakeCase(attr.Name, nil)
						tokens = append(tokens,
							&hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
							&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(camel)},
						)
					}
				}
				return tokens
			}
		}
	case "call":
		// call.resourceName.methodName[.prop...] → call(resourceName, "methodName", {args...})[.prop...]
		if len(e.Traversal) >= 3 {
			resAttr, ok1 := e.Traversal[1].(hcl.TraverseAttr)
			methodAttr, ok2 := e.Traversal[2].(hcl.TraverseAttr)
			if ok1 && ok2 {
				tokens := ft.callExprTokens(resAttr.Name, methodAttr.Name)
				for _, step := range e.Traversal[3:] {
					if attr, ok := step.(hcl.TraverseAttr); ok {
						tokens = append(tokens,
							&hclwrite.Token{Type: hclsyntax.TokenDot, Bytes: []byte(".")},
							&hclwrite.Token{Type: hclsyntax.TokenIdent, Bytes: []byte(attr.Name)},
						)
					}
				}
				return tokens
			}
		}
	case "var", "local", "module":
		return hclwrite.TokensForTraversal(stripRoot(e.Traversal))
	case "pulumi":
		if len(e.Traversal) >= 2 {
			if attr, ok := e.Traversal[1].(hcl.TraverseAttr); ok {
				switch attr.Name {
				case "stack":
					return hclwrite.TokensForFunctionCall("stack")
				case "project":
					return hclwrite.TokensForFunctionCall("project")
				case "organization":
					return hclwrite.TokensForFunctionCall("organization")
				}
			}
		}
	case "path":
		if len(e.Traversal) >= 2 {
			if attr, ok := e.Traversal[1].(hcl.TraverseAttr); ok {
				switch attr.Name {
				case "cwd":
					return hclwrite.TokensForFunctionCall("cwd")
				case "root", "module":
					return hclwrite.TokensForFunctionCall("rootDirectory")
				}
			}
		}
	}
	return hclwrite.TokensForTraversal(e.Traversal)
}

// camelCaseTraversalAttrs converts all TraverseAttr names after the root to camelCase,
// using schema-aware lookup when a cached schema is available for hclType.
// The root (logical resource name) is left unchanged.
func (ft *fileTransformer) camelCaseTraversalAttrs(trav hcl.Traversal, hclType string) hcl.Traversal {
	if len(trav) <= 1 {
		return trav
	}
	var props []*schema.Property
	if res := ft.resourceSchemas[hclType]; res != nil {
		props = res.Properties
	}
	result := make(hcl.Traversal, len(trav))
	copy(result, trav)
	for i := 1; i < len(result); i++ {
		if attr, ok := result[i].(hcl.TraverseAttr); ok {
			name, _ := transform.PulumiCaseFromSnakeCase(attr.Name, props)
			result[i] = hcl.TraverseAttr{Name: name}
		}
	}
	return result
}

// stripRoot converts a traversal like var.name.field to name.field by promoting the
// second element to the root.
func stripRoot(trav hcl.Traversal) hcl.Traversal {
	if len(trav) < 2 {
		return trav
	}
	attr, ok := trav[1].(hcl.TraverseAttr)
	if !ok {
		return trav
	}
	result := make(hcl.Traversal, len(trav)-1)
	result[0] = hcl.TraverseRoot{Name: attr.Name}
	copy(result[1:], trav[2:])
	return result
}

// callExprTokens generates PCL tokens for call(resourceName, "camelMethod", {args...}).
// It looks up the matching call block to extract the argument object.
func (ft *fileTransformer) callExprTokens(resourceName, snakeMethod string) hclwrite.Tokens {
	camelMethod, _ := transform.PulumiCaseFromSnakeCase(snakeMethod, nil)
	resTokens := hclwrite.TokensForTraversal(hcl.Traversal{hcl.TraverseRoot{Name: resourceName}})
	methodTokens := hclwrite.TokensForValue(cty.StringVal(camelMethod))

	var argsTokens hclwrite.Tokens
	if body, ok := ft.callBlocks[callReference{resourceName, snakeMethod}]; ok && len(body.Attributes) > 0 {
		var attrs []hclwrite.ObjectAttrTokens
		for _, attr := range sortedAttributes(body.Attributes) {
			name, _ := transform.PulumiCaseFromSnakeCase(attr.Name, nil)
			attrs = append(attrs, hclwrite.ObjectAttrTokens{
				Name:  hclwrite.TokensForIdentifier(name),
				Value: ft.transformExpr(attr.Expr),
			})
		}
		argsTokens = hclwrite.TokensForObject(attrs)
	} else {
		argsTokens = hclwrite.TokensForObject(nil)
	}

	return hclwrite.TokensForFunctionCall("call", resTokens, methodTokens, argsTokens)
}

// invokeExprTokens generates PCL tokens for invoke("token", {args...}) or
// invoke("token", {args...}, {opts...}) when invoke options are present.
// It looks up the matching data block to extract the argument and option objects.
func (ft *fileTransformer) invokeExprTokens(hclType, dsName string) hclwrite.Tokens {
	tokenTokens := hclwrite.TokensForValue(cty.StringVal(ft.dataTokens[hclType]))

	var argAttrs, optAttrs []hclwrite.ObjectAttrTokens
	if body, ok := ft.dataBlocks[dataReference{hclType, dsName}]; ok {
		for _, attr := range sortedAttributes(body.Attributes) {
			if pclName, isOpt := invokeOptionHCLToPCL[attr.Name]; isOpt {
				optAttrs = append(optAttrs, hclwrite.ObjectAttrTokens{
					Name:  hclwrite.TokensForIdentifier(pclName),
					Value: ft.transformExpr(attr.Expr),
				})
			} else {
				name, _ := transform.PulumiCaseFromSnakeCase(attr.Name, nil)
				argAttrs = append(argAttrs, hclwrite.ObjectAttrTokens{
					Name:  hclwrite.TokensForIdentifier(name),
					Value: ft.transformExpr(attr.Expr),
				})
			}
		}
	}

	argsTokens := hclwrite.TokensForObject(argAttrs)
	if len(optAttrs) == 0 {
		return hclwrite.TokensForFunctionCall("invoke", tokenTokens, argsTokens)
	}
	return hclwrite.TokensForFunctionCall("invoke", tokenTokens, argsTokens, hclwrite.TokensForObject(optAttrs))
}

// transformPropertyPathList converts a tuple of string literals (as used in HCL for
// replace_on_changes, ignore_changes) to a tuple of identifiers (as used in PCL for
// replaceOnChanges, ignoreChanges).
func (ft *fileTransformer) transformPropertyPathList(expr hclsyntax.Expression) hclwrite.Tokens {
	tuple, ok := expr.(*hclsyntax.TupleConsExpr)
	if !ok {
		return ft.transformExpr(expr)
	}
	var elems []hclwrite.Tokens
	for _, elem := range tuple.Exprs {
		val, diags := elem.Value(nil)
		if !diags.HasErrors() && val.Type() == cty.String {
			elems = append(elems, hclwrite.TokensForIdentifier(val.AsString()))
		} else {
			elems = append(elems, ft.transformExpr(elem))
		}
	}
	return hclwrite.TokensForTuple(elems)
}

// invertTokens inverts a boolean token expression: if tokens is "!<expr>",
// returns "<expr>"; otherwise returns "!<tokens>".
func invertTokens(tokens hclwrite.Tokens) hclwrite.Tokens {
	if len(tokens) > 0 && tokens[0].Type == hclsyntax.TokenBang {
		rest := tokens[1:]
		if len(rest) > 0 {
			rest[0].SpacesBefore = tokens[0].SpacesBefore
		}
		return rest
	}
	return append(hclwrite.Tokens{{Type: hclsyntax.TokenBang, Bytes: []byte("!")}}, tokens...)
}

// sortedKeys returns the keys of a map in sorted order.
func sortedKeys[V any](m map[string]V) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// emitPackageBlock writes a PCL "package" block from a workspace.PackageDescriptor.
func emitPackageBlock(out *hclwrite.Body, alias string, desc workspace.PackageDescriptor) {
	if desc.Parameterization == nil {
		return
	}
	blk := out.AppendNewBlock("package", []string{alias})
	blk.Body().SetAttributeValue("baseProviderName", cty.StringVal(desc.Name))
	if desc.Version != nil {
		blk.Body().SetAttributeValue("baseProviderVersion", cty.StringVal(desc.Version.String()))
	}
	paramBlk := blk.Body().AppendNewBlock("parameterization", nil)
	paramBlk.Body().SetAttributeValue("name", cty.StringVal(desc.Parameterization.Name))
	paramBlk.Body().SetAttributeValue("version", cty.StringVal(desc.Parameterization.Version.String()))
	paramBlk.Body().SetAttributeValue("value", cty.StringVal(base64.StdEncoding.EncodeToString(desc.Parameterization.Value)))
	out.AppendNewline()
}

// readParameterizationInfos reads .hcl/sdks/*/hcl.sdk.json files from dir
// and returns parameterized package descriptors keyed by alias.
func readParameterizationInfos(dir string) (map[string]workspace.PackageDescriptor, error) {
	sdksDir := filepath.Join(dir, ".hcl", "sdks")
	entries, err := os.ReadDir(sdksDir)
	if os.IsNotExist(err) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	result := make(map[string]workspace.PackageDescriptor, len(entries))
	var errs []error
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		path := filepath.Join(sdksDir, entry.Name(), "hcl.sdk.json")
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		var d workspace.PackageDescriptor
		if err := json.Unmarshal(data, &d); err != nil {
			errs = append(errs, fmt.Errorf("%q: %w", path, err))
		} else {
			result[entry.Name()] = d
		}
	}
	return result, errors.Join(errs...)
}

// transformFunctionName maps HCL function names to their PCL equivalents.
func transformFunctionName(name string) string {
	switch name {
	case "base64encode":
		return "toBase64"
	case "base64decode":
		return "fromBase64"
	case "sensitive":
		return "secret"
	case "one":
		return "singleOrNone"
	case "nonsensitive":
		return "unsecret"
	default:
		return name
	}
}
