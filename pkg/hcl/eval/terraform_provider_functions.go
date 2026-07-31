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

package eval

import (
	"errors"
	"fmt"

	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
)

// TerraformProviderFunctions returns the builtin `terraform` provider's
// provider-defined functions, keyed by TF function name. The provider ships no
// installable plugin, so these are implemented in-process and served straight
// from the eval function table instead of being projected into invokes.
func TerraformProviderFunctions() map[string]function.Function {
	return map[string]function.Function{
		"encode_expr":   encodeExprFunc,
		"encode_tfvars": encodeTFVarsFunc,
	}
}

// encodeExprFunc renders an arbitrary value as HCL expression source text. A
// wholly unknown argument short-circuits to an unknown result before Impl runs
// (AllowUnknown is false), while a known composite with unknown parts reaches
// Impl and errors. Sensitive marks are stripped from the argument and
// reapplied to the result by the function machinery.
var encodeExprFunc = function.New(&function.Spec{
	Params: []function.Parameter{{
		Name: "expr",
		Type: cty.DynamicPseudoType,
	}},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v := args[0]
		if !v.IsWhollyKnown() {
			return cty.NullVal(cty.String), errors.New("input is not wholly known")
		}
		f := hclwrite.NewEmptyFile()
		f.Body().AppendUnstructuredTokens(hclwrite.TokensForValue(v))
		return cty.StringVal(string(f.Bytes())), nil
	},
})

// encodeTFVarsFunc renders an object as .tfvars file text, one attribute per
// key in cty's sorted iteration order. Only object types are accepted — maps
// are rejected like any other non-object. A wholly unknown argument
// short-circuits to an unknown result, while a known object with unknown
// attribute values errors (matching the error text of the hclwrite panic this
// causes upstream).
var encodeTFVarsFunc = function.New(&function.Spec{
	Params: []function.Parameter{{
		Name: "input",
		Type: cty.DynamicPseudoType,
	}},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, _ cty.Type) (cty.Value, error) {
		v := args[0]
		if !v.Type().IsObjectType() {
			return cty.NullVal(cty.String), errors.New("invalid input: must be an object")
		}
		f := hclwrite.NewEmptyFile()
		body := f.Body()
		for it := v.ElementIterator(); it.Next(); {
			key, val := it.Element()
			name := key.AsString()
			if !hclsyntax.ValidIdentifier(name) {
				return cty.NullVal(cty.String),
					fmt.Errorf("invalid input: object key: %s - must be a valid identifier", name)
			}
			if !val.IsWhollyKnown() {
				return cty.NullVal(cty.String), errors.New("cannot produce tokens for unknown value")
			}
			body.SetAttributeValue(name, val)
		}
		return cty.StringVal(string(f.Bytes())), nil
	},
})
