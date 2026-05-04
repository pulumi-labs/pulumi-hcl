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

// Package comments associates source comments with the syntactic element that immediately
// follows them, so callers writing fresh [hclwrite] output can preserve comments from the
// original source.
package comments

import (
	"sort"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
)

// Map associates source comments with the syntactic element they belong to.
//
// A comment is treated as "trailing" when it begins on the same line as the
// end of an element in its enclosing body (e.g. `value = "x" // note`); such
// comments are keyed by that element's start byte so they can be emitted
// immediately after the element's value tokens.
//
// Otherwise the comment is "leading": it is assigned to the next direct
// sibling — attribute or sub-block — by finding the smallest element start
// byte in the enclosing body that is >= the comment's end byte. Comments
// with no following sibling in their scope are dropped, so they do not leak
// across block boundaries.
type Map struct {
	byElementStart  map[int]hclwrite.Tokens
	trailingByOwner map[int]hclwrite.Tokens
}

// Build lexes src and returns a Map that assigns each source comment to the
// element in body that immediately follows it.
//
// Block types listed in skipBlockTypes are not treated as anchors: comments
// preceding such a block (or appearing inside it) flow past it to the next
// non-skipped sibling. This is useful when the caller does not emit those
// blocks in its output (for example, the converter skips `data` and `call`
// blocks; codegen skips `package` blocks).
type bodyScope struct {
	body          *hclsyntax.Body
	startByte     int         // inclusive byte where this scope begins
	endByte       int         // exclusive byte where this scope ends
	starts        []int       // direct-child element start bytes (sorted)
	ends          map[int]int // start byte → end byte, for direct children
	endsByEndLine map[int]int // end line → element start byte, for direct children
}

func Build(src []byte, filename string, body *hclsyntax.Body, skipBlockTypes ...string) *Map {
	m := &Map{
		byElementStart:  map[int]hclwrite.Tokens{},
		trailingByOwner: map[int]hclwrite.Tokens{},
	}

	tokens, diags := hclsyntax.LexConfig(src, filename, hcl.Pos{Byte: 0, Line: 1, Column: 1})
	if diags.HasErrors() {
		return m
	}

	skip := make(map[string]bool, len(skipBlockTypes))
	for _, t := range skipBlockTypes {
		skip[t] = true
	}

	var scopes []bodyScope
	collectScopes(body, skip, &scopes)
	for i := range scopes {
		sort.Ints(scopes[i].starts)
	}
	// The root body's SrcRange can start past leading trivia (block comments
	// at the start of a file), but we want comments anywhere in the source to
	// associate with elements in the root scope. Extend the root scope to
	// cover the full source extent.
	if len(scopes) > 0 {
		scopes[0].startByte = 0
		scopes[0].endByte = len(src)
	}

	for _, t := range tokens {
		if t.Type != hclsyntax.TokenComment {
			continue
		}
		scope := innermostScope(scopes, t.Range.Start.Byte)
		if scope == nil {
			continue
		}
		// Trailing line comment: starts on the same line as a direct child's end.
		if owner, ok := scope.endsByEndLine[t.Range.Start.Line]; ok &&
			t.Range.Start.Byte >= scope.ends[owner] {
			m.trailingByOwner[owner] = append(m.trailingByOwner[owner], trailingCommentTokenFor(t))
			continue
		}
		idx := sort.SearchInts(scope.starts, t.Range.End.Byte)
		if idx >= len(scope.starts) {
			// No following sibling in this scope; drop the comment.
			continue
		}
		key := scope.starts[idx]
		m.byElementStart[key] = append(m.byElementStart[key], commentTokenFor(t))
	}

	return m
}

// Leading returns the leading comments associated with the element starting at
// the given source byte and removes them from the map so each comment is
// emitted at most once.
func (m *Map) Leading(startByte int) hclwrite.Tokens {
	if m == nil {
		return nil
	}
	tokens := m.byElementStart[startByte]
	if len(tokens) == 0 {
		return nil
	}
	delete(m.byElementStart, startByte)
	return tokens
}

// Emit appends the leading comments for the element at startByte to body.
func (m *Map) Emit(body *hclwrite.Body, startByte int) {
	tokens := m.Leading(startByte)
	if len(tokens) == 0 {
		return
	}
	body.AppendUnstructuredTokens(tokens)
}

// Trailing returns the same-line trailing comments associated with the element
// starting at the given source byte and removes them so each is emitted at
// most once. Callers should append the returned tokens to the attribute's
// value tokens before passing them to hclwrite.Body.SetAttributeRaw, so the
// comment ends up on the same line as the value rather than after the
// attribute's auto-emitted newline.
func (m *Map) Trailing(startByte int) hclwrite.Tokens {
	if m == nil {
		return nil
	}
	tokens := m.trailingByOwner[startByte]
	if len(tokens) == 0 {
		return nil
	}
	delete(m.trailingByOwner, startByte)
	return tokens
}

// AppendTrailing returns valueTokens with the trailing comment for the element
// at startByte (if any) appended. The result is suitable for
// hclwrite.Body.SetAttributeRaw so the comment lands on the same line as the
// value.
func (m *Map) AppendTrailing(valueTokens hclwrite.Tokens, startByte int) hclwrite.Tokens {
	trailing := m.Trailing(startByte)
	if len(trailing) == 0 {
		return valueTokens
	}
	return append(valueTokens, trailing...)
}

// commentTokenFor converts a lexed hclsyntax comment token into an hclwrite
// token. Line comments (`//` and `#`) include a trailing newline already;
// block comments (`/* ... */`) do not, so we add one to keep the next element
// on its own line.
func commentTokenFor(t hclsyntax.Token) *hclwrite.Token {
	bytes := t.Bytes
	if len(bytes) == 0 || bytes[len(bytes)-1] != '\n' {
		buf := make([]byte, len(bytes)+1)
		copy(buf, bytes)
		buf[len(buf)-1] = '\n'
		bytes = buf
	}
	return &hclwrite.Token{Type: hclsyntax.TokenComment, Bytes: bytes}
}

// trailingCommentTokenFor builds a trailing-comment token to follow an
// attribute's value tokens. SpacesBefore=1 keeps a single space between the
// value and the comment (e.g. `value = "x" // note`). Any trailing newline on
// the lexed comment is stripped because SetAttributeRaw will emit its own
// terminating newline; leaving the lexer's `\n` in place would produce a
// stray blank line below the attribute.
func trailingCommentTokenFor(t hclsyntax.Token) *hclwrite.Token {
	bytes := t.Bytes
	for len(bytes) > 0 && bytes[len(bytes)-1] == '\n' {
		bytes = bytes[:len(bytes)-1]
	}
	return &hclwrite.Token{
		Type:         hclsyntax.TokenComment,
		Bytes:        append([]byte(nil), bytes...),
		SpacesBefore: 1,
	}
}

// collectScopes walks body recursively and records each body and its direct
// attribute/block start bytes. Skipped block types are not recorded as anchors
// in their parent scope, but their bodies are still walked so that comments
// inside them associate with siblings inside the skipped block (or are
// dropped) rather than leaking out to the surrounding scope.
func collectScopes(body *hclsyntax.Body, skip map[string]bool, scopes *[]bodyScope) {
	scope := bodyScope{
		body:          body,
		startByte:     body.SrcRange.Start.Byte,
		endByte:       body.SrcRange.End.Byte,
		ends:          map[int]int{},
		endsByEndLine: map[int]int{},
	}
	addChild := func(rng hcl.Range) {
		scope.starts = append(scope.starts, rng.Start.Byte)
		scope.ends[rng.Start.Byte] = rng.End.Byte
		scope.endsByEndLine[rng.End.Line] = rng.Start.Byte
	}
	for _, attr := range body.Attributes {
		addChild(attr.Range())
	}
	for _, block := range body.Blocks {
		if skip[block.Type] {
			continue
		}
		addChild(block.Range())
	}
	*scopes = append(*scopes, scope)
	for _, block := range body.Blocks {
		collectScopes(block.Body, skip, scopes)
	}
}

// innermostScope returns the bodyScope whose extent contains the given byte
// offset and is the deepest such scope, or nil if no scope contains it.
func innermostScope(scopes []bodyScope, byte int) *bodyScope {
	var best *bodyScope
	for i := range scopes {
		if byte >= scopes[i].startByte && byte < scopes[i].endByte {
			if best == nil || scopes[i].startByte > best.startByte {
				best = &scopes[i]
			}
		}
	}
	return best
}
