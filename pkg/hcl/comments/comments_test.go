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

package comments

import (
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/hashicorp/hcl/v2/hclwrite"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// parse parses src as HCL and returns its body. Fails the test on errors.
func parse(t *testing.T, src string) *hclsyntax.Body {
	t.Helper()
	file, diags := hclsyntax.ParseConfig([]byte(src), "test.hcl", hcl.Pos{Byte: 0, Line: 1, Column: 1})
	require.False(t, diags.HasErrors(), "parse: %v", diags)
	return file.Body.(*hclsyntax.Body)
}

// buildAndEmit builds a Map from src and writes the leading + trailing comments
// for each top-level attribute (in source order) of body into a fresh hclwrite
// file, returning the rendered bytes.
func buildAndEmit(t *testing.T, src string, skip ...string) string {
	t.Helper()
	body := parse(t, src)
	m := Build([]byte(src), "test.hcl", body, skip...)

	out := hclwrite.NewEmptyFile()
	for _, attr := range body.Attributes {
		start := attr.Range().Start.Byte
		m.Emit(out.Body(), start)
		out.Body().SetAttributeRaw(attr.Name, m.AppendTrailing(
			hclwrite.Tokens{{Type: hclsyntax.TokenIdent, Bytes: []byte("PLACEHOLDER")}},
			start,
		))
	}
	return string(out.Bytes())
}

func TestBuild_LeadingAttachedToNextSibling(t *testing.T) {
	t.Parallel()

	src := "// preamble\nfoo = 1\n"
	got := buildAndEmit(t, src)
	assert.Equal(t, "// preamble\nfoo = PLACEHOLDER\n", got)
}

func TestBuild_TrailingStaysOnSameLine(t *testing.T) {
	t.Parallel()

	src := "foo = 1 // note\n"
	got := buildAndEmit(t, src)
	assert.Equal(t, "foo = PLACEHOLDER // note\n", got)
}

func TestBuild_BlockCommentTrailingNewlineAdded(t *testing.T) {
	t.Parallel()

	// Block comments have no trailing newline in the lexer; commentTokenFor
	// should add one so the next element starts on its own line.
	src := "/* preamble */\nfoo = 1\n"
	got := buildAndEmit(t, src)
	assert.Equal(t, "/* preamble */\nfoo = PLACEHOLDER\n", got)
}

func TestBuild_DroppedWhenNoFollowingSibling(t *testing.T) {
	t.Parallel()

	// The comment has no following sibling, so it must not leak; the output
	// has no comment at all.
	src := "foo = 1\n// trailing-of-file\n"
	got := buildAndEmit(t, src)
	assert.Equal(t, "foo = PLACEHOLDER\n", got)
}

func TestBuild_ScopedToEnclosingBlock(t *testing.T) {
	t.Parallel()

	// A trailing comment inside a block must not leak to the next sibling
	// outside the block.
	src := "block {\n  inner = 1 // note\n}\nouter = 2\n"
	body := parse(t, src)
	m := Build([]byte(src), "test.hcl", body)

	// The outer attribute should have NO leading comment associated with it.
	outerStart := body.Attributes["outer"].Range().Start.Byte
	assert.Empty(t, m.Leading(outerStart))
	assert.Empty(t, m.Trailing(outerStart))

	// The inner attribute's trailing comment is preserved.
	innerStart := body.Blocks[0].Body.Attributes["inner"].Range().Start.Byte
	trailing := m.Trailing(innerStart)
	require.Len(t, trailing, 1)
	assert.Equal(t, "// note", string(trailing[0].Bytes))
}

func TestBuild_SkipBlockTypesFlowsCommentsPast(t *testing.T) {
	t.Parallel()

	// "skipme" is excluded as an anchor in its parent scope, so the leading
	// comment lands on the next non-skipped sibling instead.
	src := "// preamble\nskipme {}\nfoo = 1\n"
	body := parse(t, src)
	m := Build([]byte(src), "test.hcl", body, "skipme")

	out := hclwrite.NewEmptyFile()
	m.Emit(out.Body(), body.Attributes["foo"].Range().Start.Byte)
	out.Body().SetAttributeRaw("foo", hclwrite.Tokens{
		{Type: hclsyntax.TokenIdent, Bytes: []byte("PLACEHOLDER")},
	})
	assert.Equal(t, "// preamble\nfoo = PLACEHOLDER\n", string(out.Bytes()))
}

func TestBuild_LexErrorReturnsEmptyMap(t *testing.T) {
	t.Parallel()

	// An unterminated string makes hclsyntax.LexConfig produce diagnostics;
	// Build should bail out early and return an empty Map without panicking.
	body := &hclsyntax.Body{
		SrcRange: hcl.Range{
			Filename: "test.hcl",
			Start:    hcl.Pos{Byte: 0, Line: 1, Column: 1},
			End:      hcl.Pos{Byte: 0, Line: 1, Column: 1},
		},
	}
	m := Build([]byte("\""), "test.hcl", body)
	require.NotNil(t, m)
	assert.Empty(t, m.Leading(0))
	assert.Empty(t, m.Trailing(0))
}

func TestNilMap_IsSafe(t *testing.T) {
	t.Parallel()

	// All public accessors must tolerate a nil receiver so callers can use
	// (*Map)(nil) freely.
	var m *Map
	assert.Nil(t, m.Leading(0))
	assert.Nil(t, m.Trailing(0))
	assert.Equal(t, hclwrite.Tokens(nil), m.AppendTrailing(nil, 0))

	out := hclwrite.NewEmptyFile()
	m.Emit(out.Body(), 0)
	assert.Empty(t, string(out.Bytes()))
}

func TestEmit_NoOpWhenNoComments(t *testing.T) {
	t.Parallel()

	body := parse(t, "foo = 1\n")
	m := Build([]byte("foo = 1\n"), "test.hcl", body)

	out := hclwrite.NewEmptyFile()
	m.Emit(out.Body(), body.Attributes["foo"].Range().Start.Byte)
	assert.Empty(t, string(out.Bytes()))
}

func TestAppendTrailing_NoCommentReturnsInput(t *testing.T) {
	t.Parallel()

	body := parse(t, "foo = 1\n")
	m := Build([]byte("foo = 1\n"), "test.hcl", body)

	in := hclwrite.Tokens{{Type: hclsyntax.TokenIdent, Bytes: []byte("X")}}
	got := m.AppendTrailing(in, body.Attributes["foo"].Range().Start.Byte)
	assert.Equal(t, in, got)
}

func TestLeadingAndTrailing_EmittedOnce(t *testing.T) {
	t.Parallel()

	body := parse(t, "// lead\nfoo = 1 // tail\n")
	m := Build([]byte("// lead\nfoo = 1 // tail\n"), "test.hcl", body)

	start := body.Attributes["foo"].Range().Start.Byte
	require.Len(t, m.Leading(start), 1)
	require.Len(t, m.Trailing(start), 1)
	// Second call must return nil — comments are removed once emitted.
	assert.Empty(t, m.Leading(start))
	assert.Empty(t, m.Trailing(start))
}
