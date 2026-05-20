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

package packages

import (
	"strings"
	"unicode"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// suggestEditDistanceThreshold is the maximum edit distance between the user's
// HCL token and a candidate token from the schema for which we will surface a
// "did you mean" suggestion. Picked empirically so a one-character typo on a
// short member name still triggers, while completely unrelated names do not.
const suggestEditDistanceThreshold = 3

// nearestHCLToken returns the HCL form of the schema token in pkg closest to
// hclToken under Levenshtein distance, or "" if no candidate is within
// suggestEditDistanceThreshold. isFunction selects pkg.Functions() over
// pkg.Resources(). Callers must only invoke this when pkg has been
// successfully loaded but a specific resource/function lookup failed.
func nearestHCLToken(pkg schema.PackageReference, hclToken string, isFunction bool) string {
	bestDist := suggestEditDistanceThreshold + 1
	var best string
	visit := func(tok string) {
		candidate := pulumiTokenToHCLForm(pkg, tok, isFunction)
		if candidate == "" {
			return
		}
		d := levenshtein(hclToken, candidate)
		if d < bestDist {
			bestDist = d
			best = candidate
		}
	}
	if isFunction {
		for iter := pkg.Functions().Range(); iter.Next(); {
			visit(iter.Token())
		}
	} else {
		for iter := pkg.Resources().Range(); iter.Next(); {
			visit(iter.Token())
		}
	}
	if bestDist <= suggestEditDistanceThreshold {
		return best
	}
	return ""
}

// pulumiTokenToHCLForm converts a Pulumi token (e.g. "aws:ec2/vpc:Vpc") to
// the HCL form that the resolver would accept (e.g. "aws_ec2_vpc"). It uses
// pkg.TokenToModule so bridged-provider tokens (with their schema
// ModuleFormat regex) are normalized correctly.
func pulumiTokenToHCLForm(pkg schema.PackageReference, token string, isFunction bool) string {
	parts := strings.SplitN(token, ":", 3)
	if len(parts) < 3 {
		return ""
	}
	pkgName := parts[0]
	name := parts[2]
	mod := pkg.TokenToModule(token)

	if isFunction && strings.HasPrefix(name, "get") && len(name) > 3 {
		r := rune(name[3])
		if r >= 'A' && r <= 'Z' {
			name = name[3:]
		}
	}

	var b strings.Builder
	b.WriteString(pkgName)
	if mod != "" && mod != "index" {
		b.WriteRune('_')
		b.WriteString(strings.ToLower(strings.ReplaceAll(mod, "/", "_")))
	}
	b.WriteRune('_')
	b.WriteString(camelToSnake(name))
	return b.String()
}

func camelToSnake(s string) string {
	var b strings.Builder
	for i, r := range s {
		if i > 0 && unicode.IsUpper(r) {
			prev := rune(s[i-1])
			if prev != '_' && prev != '/' {
				b.WriteRune('_')
			}
		}
		if r == '/' {
			b.WriteRune('_')
		} else {
			b.WriteRune(unicode.ToLower(r))
		}
	}
	return b.String()
}

// levenshtein computes the edit distance between two ASCII-ish strings using
// the standard two-row dynamic-programming variant.
func levenshtein(a, b string) int {
	if a == b {
		return 0
	}
	if len(a) == 0 {
		return len(b)
	}
	if len(b) == 0 {
		return len(a)
	}
	prev := make([]int, len(b)+1)
	curr := make([]int, len(b)+1)
	for j := range prev {
		prev[j] = j
	}
	for i := 1; i <= len(a); i++ {
		curr[0] = i
		for j := 1; j <= len(b); j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			curr[j] = min(curr[j-1]+1, prev[j]+1, prev[j-1]+cost)
		}
		prev, curr = curr, prev
	}
	return prev[len(b)]
}
