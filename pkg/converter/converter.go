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
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/plugin"
	"github.com/pulumi/pulumi/sdk/v3/go/common/util/contract"
	"gopkg.in/yaml.v3"
)

type hclConverter struct{}

// New returns a new HCL→PCL converter.
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

		pcl, diags, err := transformHCLToPCL(content, entry.Name())
		if err != nil {
			return nil, err
		}
		allDiags = append(allDiags, diags...)

		dstName := strings.TrimSuffix(entry.Name(), ".hcl") + ".pp"
		if err := os.WriteFile(filepath.Join(req.TargetDirectory, dstName), pcl, 0o644); err != nil {
			return nil, fmt.Errorf("writing %s: %w", dstName, err)
		}
	}

	return &plugin.ConvertProgramResponse{Diagnostics: allDiags}, nil
}

// transformHCLToPCL converts HCL source bytes to PCL source bytes.
// It returns any non-fatal diagnostics (e.g., unsupported block types).
func transformHCLToPCL(src []byte, filename string) ([]byte, hcl.Diagnostics, error) {
	file, diags := hclsyntax.ParseConfig(src, filename, hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		return nil, diags, nil
	}

	body := file.Body.(*hclsyntax.Body)

	var out bytes.Buffer
	var resultDiags hcl.Diagnostics

	for _, block := range body.Blocks {
		switch block.Type {

		case "terraform":
			// Skip: no PCL equivalent.

		case "variable":
			if len(block.Labels) == 0 {
				continue
			}
			name := block.Labels[0]
			typeStr := "string"
			if typeAttr, ok := block.Body.Attributes["type"]; ok {
				typeStr = convertHCLTypeExpr(src, typeAttr.Expr)
			}
			fmt.Fprintf(&out, "config %q %q {}\n\n", name, typeStr)

		case "locals":
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				exprBytes := transformExpr(src, attr.Expr)
				fmt.Fprintf(&out, "%s = %s\n\n", attr.Name, exprBytes)
			}

		case "output":
			if len(block.Labels) == 0 {
				continue
			}
			name := block.Labels[0]
			fmt.Fprintf(&out, "output %q {\n", name)
			for _, attr := range sortedAttributes(block.Body.Attributes) {
				exprBytes := transformExpr(src, attr.Expr)
				fmt.Fprintf(&out, "  %s = %s\n", attr.Name, exprBytes)
			}
			fmt.Fprintf(&out, "}\n\n")

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
		exprBytes := transformExpr(src, attr.Expr)
		fmt.Fprintf(&out, "%s = %s\n\n", attr.Name, exprBytes)
	}

	return out.Bytes(), resultDiags, nil
}

// sortedAttributes returns attributes sorted by source position.
func sortedAttributes(attrs hclsyntax.Attributes) []*hclsyntax.Attribute {
	result := make([]*hclsyntax.Attribute, 0, len(attrs))
	for _, attr := range attrs {
		result = append(result, attr)
	}
	sort.Slice(result, func(i, j int) bool {
		return result[i].NameRange.Start.Byte < result[j].NameRange.Start.Byte
	})
	return result
}

// convertHCLTypeExpr converts an HCL type constraint expression to a PCL type string.
func convertHCLTypeExpr(src []byte, expr hclsyntax.Expression) string {
	switch e := expr.(type) {
	case *hclsyntax.ScopeTraversalExpr:
		switch e.Traversal.RootName() {
		case "string":
			return "string"
		case "bool":
			return "bool"
		case "number":
			return "number"
		case "any":
			return "any"
		}
	case *hclsyntax.FunctionCallExpr:
		switch e.Name {
		case "list":
			if len(e.Args) == 1 {
				return "List<" + convertHCLTypeExpr(src, e.Args[0]) + ">"
			}
		case "map":
			if len(e.Args) == 1 {
				return "Map<" + convertHCLTypeExpr(src, e.Args[0]) + ">"
			}
		case "set":
			if len(e.Args) == 1 {
				return "Set<" + convertHCLTypeExpr(src, e.Args[0]) + ">"
			}
		case "object", "tuple":
			return "any"
		}
	}
	// Fallback: use source bytes.
	return string(src[expr.Range().Start.Byte:expr.Range().End.Byte])
}

// transformExpr applies expression rewrites to the source bytes spanning the expression.
func transformExpr(src []byte, expr hclsyntax.Expression) []byte {
	edits, diags := collectEdits(expr)
	contract.Assertf(!diags.HasErrors(), "unexpected errors: %s", diags)
	offset := expr.Range().Start.Byte
	exprSrc := src[offset:expr.Range().End.Byte]

	// Adjust edit positions to be relative to exprSrc.
	adjusted := make([]edit, len(edits))
	for i, e := range edits {
		adjusted[i] = edit{e.start - offset, e.end - offset, e.text}
	}
	return applyEdits(exprSrc, adjusted)
}

// edit describes a text replacement in a byte slice.
type edit struct {
	start int
	end   int
	text  string
}

// collectEdits walks an expression AST and returns all rewrites needed for PCL.
func collectEdits(expr hclsyntax.Expression) ([]edit, hcl.Diagnostics) {
	var edits []edit
	diags := hclsyntax.VisitAll(expr, func(node hclsyntax.Node) hcl.Diagnostics {
		switch e := node.(type) {
		case *hclsyntax.ScopeTraversalExpr:
			edits = append(edits, traversalEdits(e)...)
		case *hclsyntax.FunctionCallExpr:
			edits = append(edits, functionEdits(e)...)
		}
		return nil
	})
	return edits, diags
}

// traversalEdits returns edits for HCL scope traversal expressions.
func traversalEdits(e *hclsyntax.ScopeTraversalExpr) []edit {
	start := e.SrcRange.Start.Byte
	end := e.SrcRange.End.Byte
	switch e.Traversal.RootName() {
	case "var":
		// Remove "var." prefix: var.name → name
		return []edit{{start, start + len("var."), ""}}
	case "local":
		// Remove "local." prefix: local.name → name
		return []edit{{start, start + len("local."), ""}}
	case "pulumi":
		if len(e.Traversal) >= 2 {
			if attr, ok := e.Traversal[1].(hcl.TraverseAttr); ok {
				switch attr.Name {
				case "stack":
					return []edit{{start, end, "stack()"}}
				case "project":
					return []edit{{start, end, "project()"}}
				case "organization":
					return []edit{{start, end, "organization()"}}
				}
			}
		}
	case "path":
		if len(e.Traversal) >= 2 {
			if attr, ok := e.Traversal[1].(hcl.TraverseAttr); ok {
				switch attr.Name {
				case "cwd":
					return []edit{{start, end, "cwd()"}}
				case "root", "module":
					return []edit{{start, end, "rootDirectory()"}}
				}
			}
		}
	}
	return nil
}

// functionEdits returns edits for renamed HCL built-in functions.
func functionEdits(e *hclsyntax.FunctionCallExpr) []edit {
	start := e.NameRange.Start.Byte
	end := e.NameRange.End.Byte
	switch e.Name {
	case "base64encode":
		return []edit{{start, end, "toBase64"}}
	case "base64decode":
		return []edit{{start, end, "fromBase64"}}
	case "sensitive":
		return []edit{{start, end, "secret"}}
	case "one":
		return []edit{{start, end, "singleOrNone"}}
	}
	return nil
}

// applyEdits applies a set of non-overlapping edits to src, returning the result.
// Edits must use positions within src (already adjusted to be relative to src).
func applyEdits(src []byte, edits []edit) []byte {
	if len(edits) == 0 {
		return src
	}
	sort.Slice(edits, func(i, j int) bool {
		return edits[i].start < edits[j].start
	})

	var buf bytes.Buffer
	pos := 0
	for _, e := range edits {
		if e.start < pos {
			// Skip overlapping edits (shouldn't happen in practice).
			continue
		}
		buf.Write(src[pos:e.start])
		buf.WriteString(e.text)
		pos = e.end
	}
	buf.Write(src[pos:])
	return buf.Bytes()
}
