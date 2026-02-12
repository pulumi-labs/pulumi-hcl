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
	switch e := expr.(type) {
	case *model.FunctionCallExpression:
		return genFunctionCall(body, name, e)
	case *model.LiteralValueExpression:
		body.SetAttributeValue(name, e.Value)
		return nil
	case *model.TemplateExpression:
		if len(e.Parts) == 1 {
			return genExpression(body, name, e.Parts[0])
		}
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	default:
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported expression type",
			Detail:   fmt.Sprintf("expression type %T is not yet supported", expr),
		}}
	}
}

func genFunctionCall(body *hclwrite.Body, name string, expr *model.FunctionCallExpression) hcl.Diagnostics {
	switch expr.Name {
	case "cwd":
		body.SetAttributeTraversal(name, hcl.Traversal{
			hcl.TraverseRoot{Name: "path"},
			hcl.TraverseAttr{Name: "cwd"},
		})
		return nil
	case "stack":
		body.SetAttributeTraversal(name, hcl.Traversal{
			hcl.TraverseRoot{Name: "terraform"},
			hcl.TraverseAttr{Name: "workspace"},
		})
		return nil
	default:
		return hcl.Diagnostics{{
			Severity: hcl.DiagError,
			Summary:  "unsupported function",
			Detail:   fmt.Sprintf("function %q is not yet supported", expr.Name),
		}}
	}
}

