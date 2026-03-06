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
	"github.com/pulumi/pulumi-language-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
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
	_ context.Context, req *plugin.ConvertProgramRequest,
) (*plugin.ConvertProgramResponse, error) {
	if req.LoaderTarget == "" {
		return nil, fmt.Errorf("missing loader address")
	}

	client, err := schema.NewLoaderClient(req.LoaderTarget)
	if err != nil {
		return nil, fmt.Errorf("creating loader client: %w", err)
	}
	defer contract.IgnoreClose(client)
	loader := schema.NewCachedLoader(client)

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

	// Walk source dir for .hcl files, transform each to a .pp PCL file.
	entries, err := os.ReadDir(req.SourceDirectory)
	if err != nil {
		return nil, fmt.Errorf("reading source directory: %w", err)
	}

	var allDiags hcl.Diagnostics

	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".hcl") {
			continue
		}

		content, err := os.ReadFile(filepath.Join(req.SourceDirectory, entry.Name()))
		if err != nil {
			return nil, fmt.Errorf("reading %s: %w", entry.Name(), err)
		}

		out := hclwrite.NewEmptyFile()
		diags, err := transformHCLFileToPCL(content, entry.Name(), out.Body(), loader)
		if err != nil {
			return nil, err
		}
		allDiags = append(allDiags, diags...)

		dstName := strings.TrimSuffix(entry.Name(), ".hcl") + ".pp"
		f, err := os.Create(filepath.Join(req.TargetDirectory, dstName))
		if err != nil {
			return nil, fmt.Errorf("opening %s: %w", dstName, err)
		}
		defer contract.IgnoreClose(f)
		if _, err := out.WriteTo(f); err != nil {
			return nil, fmt.Errorf("writing to %s: %w", dstName, err)
		}
	}

	return &plugin.ConvertProgramResponse{Diagnostics: allDiags}, nil
}

// fileTransformer holds context for converting a single HCL file to PCL.
type fileTransformer struct {
	src           []byte
	knownHCLTypes map[string]bool // set of HCL type labels used in resource blocks
	loader        schema.ReferenceLoader
}

// newFileTransformer creates a fileTransformer by pre-scanning body for resource definitions.
func newFileTransformer(src []byte, body *hclsyntax.Body, loader schema.ReferenceLoader) *fileTransformer {
	ft := &fileTransformer{
		src:           src,
		knownHCLTypes: make(map[string]bool),
		loader:        loader,
	}
	for _, block := range body.Blocks {
		if block.Type == "resource" && len(block.Labels) >= 1 {
			ft.knownHCLTypes[block.Labels[0]] = true
		}
	}
	return ft
}

// hclTypeToPCLToken converts an HCL resource type (e.g., "pulumi_stash") to a PCL
// token (e.g., "pulumi:index:Stash") using schema resolution.
func (ft *fileTransformer) hclTypeToPCLToken(hclType string) (string, error) {
	res, err := packages.ResolveResource(context.Background(), ft.loader, nil, hclType)
	if err != nil {
		return "", fmt.Errorf("resolving resource type %q: %w", hclType, err)
	}
	return res.Token, nil
}

// transformHCLFileToPCL converts HCL source bytes to PCL source bytes.
// It returns any non-fatal diagnostics (e.g., unsupported block types).
func transformHCLFileToPCL(
	src []byte, filename string, out *hclwrite.Body, loader schema.ReferenceLoader,
) (hcl.Diagnostics, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return diags, nil
	}

	body := file.Body.(*hclsyntax.Body)
	ft := newFileTransformer(src, body, loader)

	var resultDiags hcl.Diagnostics

	for _, block := range body.Blocks {
		switch block.Type {

		case "terraform":
			// Skip: no PCL equivalent.

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

			pclToken, err := ft.hclTypeToPCLToken(hclType)
			if err != nil {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "unknown resource type",
					Detail:   fmt.Sprintf("cannot convert HCL type %q to PCL token: %v", hclType, err),
					Subject:  block.TypeRange.Ptr(),
				})
				continue
			}

			blk := out.AppendNewBlock("resource", []string{logicalName, pclToken})
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				blk.Body().SetAttributeRaw(attr.Name, ft.transformExpr(attr.Expr))
			}
			if len(block.Body.Blocks) > 0 {
				resultDiags = append(resultDiags, &hcl.Diagnostic{
					Severity: hcl.DiagError,
					Summary:  "unsupported resource sub-blocks",
					Detail:   "nested blocks within resource blocks are not yet supported by the HCL converter",
					Subject:  block.TypeRange.Ptr(),
				})
			}
			out.AppendNewline()

		case "pulumi":
			blk := out.AppendNewBlock("pulumi", nil)
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				blk.Body().SetAttributeRaw(snakeToCamel(attr.Name), ft.transformExpr(attr.Expr))
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

// snakeToCamel converts a snake_case identifier to camelCase.
func snakeToCamel(s string) string {
	parts := strings.Split(s, "_")
	for i := 1; i < len(parts); i++ {
		if len(parts[i]) > 0 {
			parts[i] = strings.ToUpper(parts[i][:1]) + parts[i][1:]
		}
	}
	return strings.Join(parts, "")
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

	// Resource traversal: strip the HCL type prefix (e.g., "pulumi_stash.myRes.prop" → "myRes.prop").
	if ft.knownHCLTypes[root] {
		return hclwrite.TokensForTraversal(stripRoot(e.Traversal))
	}

	switch root {
	case "var", "local":
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
