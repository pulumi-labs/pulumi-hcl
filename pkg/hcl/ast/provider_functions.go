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

package ast

import (
	"maps"
	"slices"
	"strings"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
)

// Provider-defined function calls appear in expressions as
// provider::<localname>::<function>(...). HCL parses the namespaced name into
// a single flat function-table key, so discovering them is a syntax-tree scan
// for function calls whose name carries the provider:: prefix. The parser
// records the per-module aggregate on Config; the dependency graph scans each
// expression it already walks.

const providerFunctionPrefix = "provider::"

// ParseProviderFunctionName splits a function-table key like
// "provider::aws::arn_parse" into its provider local name ("aws") and function
// name ("arn_parse"). ok is false when name is not a provider-defined function
// call name.
func ParseProviderFunctionName(name string) (providerName, funcName string, ok bool) {
	rest, found := strings.CutPrefix(name, providerFunctionPrefix)
	if !found {
		return "", "", false
	}
	providerName, funcName, found = strings.Cut(rest, "::")
	if !found || providerName == "" || funcName == "" || strings.Contains(funcName, "::") {
		return "", "", false
	}
	return providerName, funcName, true
}

// ProviderFunctionName builds the function-table key for a provider-defined
// function, inverting ParseProviderFunctionName.
func ProviderFunctionName(providerName, funcName string) string {
	return providerFunctionPrefix + providerName + "::" + funcName
}

// ProviderFunctionCallsInExpr returns the provider local names referenced by
// provider-defined function calls anywhere in expr, deduplicated and sorted.
// Only native-syntax expressions can be scanned; other expression
// implementations (JSON syntax) yield nil.
func ProviderFunctionCallsInExpr(expr hcl.Expression) []string {
	node, ok := expr.(hclsyntax.Node)
	if !ok {
		return nil
	}
	return collectProviderFunctionCalls(node)
}

// ProviderFunctionCallsInBody returns the provider local names referenced by
// provider-defined function calls anywhere in body, deduplicated and sorted.
// ok reports whether the body could be scanned: JSON-syntax bodies cannot,
// and the caller must fall back to assuming any declared provider's functions
// may be called.
func ProviderFunctionCallsInBody(body hcl.Body) (calls []string, ok bool) {
	node, isNative := body.(*hclsyntax.Body)
	if !isNative {
		return nil, false
	}
	return collectProviderFunctionCalls(node), true
}

func collectProviderFunctionCalls(node hclsyntax.Node) []string {
	set := map[string]struct{}{}
	// The visitor returns no diagnostics, so VisitAll cannot either.
	_ = hclsyntax.VisitAll(node, func(n hclsyntax.Node) hcl.Diagnostics {
		if call, ok := n.(*hclsyntax.FunctionCallExpr); ok {
			if providerName, _, ok := ParseProviderFunctionName(call.Name); ok {
				set[providerName] = struct{}{}
			}
		}
		return nil
	})
	if len(set) == 0 {
		return nil
	}
	return slices.Sorted(maps.Keys(set))
}
