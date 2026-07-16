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

package modulepath_test

import (
	"slices"
	"strings"
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/pkg/hcl/modulepath"
	"github.com/stretchr/testify/assert"
)

func TestRoot(t *testing.T) {
	t.Parallel()

	r := modulepath.Root()
	assert.True(t, r.IsRoot())
	assert.Equal(t, 0, r.Len())
	assert.Equal(t, "", r.LogicalName())
	assert.Equal(t, "", r.String())
}

func TestStep_LogicalName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name string
		step modulepath.Step
		want string
	}{
		{"bare", modulepath.NewStep("vpc"), "vpc"},
		{"bare-with-dot", modulepath.NewStep("vpc.primary"), "vpc.primary"},
		{"index-zero", modulepath.NewIndexedStep("vpc", 0), "vpc[0]"},
		{"index-large", modulepath.NewIndexedStep("vpc", 17), "vpc[17]"},
		{"each-string", modulepath.NewKeyedStep("vpc", "prod"), `vpc["prod"]`},
		{"each-empty", modulepath.NewKeyedStep("vpc", ""), `vpc[""]`},
		{"each-with-dash", modulepath.NewKeyedStep("vpc", "a-b"), `vpc["a-b"]`},
		{"each-with-quote", modulepath.NewKeyedStep("vpc", `a"b`), `vpc["a\"b"]`},
	}
	for _, tt := range cases {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, tt.want, tt.step.LogicalName())
		})
	}
}

func TestStep_Accessors(t *testing.T) {
	t.Parallel()

	bare := modulepath.NewStep("a")
	assert.Equal(t, "a", bare.Name())
	_, ok := bare.Key()
	assert.False(t, ok)

	key := modulepath.NewKeyedStep("a", "k")
	gotKey, ok := key.Key()
	assert.True(t, ok)
	assert.Equal(t, "k", gotKey)
}

func TestNewIndexedStep_NegativePanics(t *testing.T) {
	t.Parallel()

	assert.Panics(t, func() { modulepath.NewIndexedStep("a", -1) })
}

func TestPath_Append(t *testing.T) {
	t.Parallel()

	p := modulepath.Root().Append(modulepath.NewStep("a")).Append(modulepath.NewStep("b"))
	assert.Equal(t, 2, p.Len())
	assert.False(t, p.IsRoot())
	assert.Equal(t, "a.b", p.LogicalName())
}

func TestPath_AppendDoesNotMutate(t *testing.T) {
	t.Parallel()

	p1 := modulepath.Root().Append(modulepath.NewStep("a"))
	p2 := p1.Append(modulepath.NewStep("b"))
	// p1 must still be just "a".
	assert.Equal(t, "a", p1.LogicalName())
	assert.Equal(t, "a.b", p2.LogicalName())
}

func TestPath_LogicalName_RoundTripsDottedLabels(t *testing.T) {
	t.Parallel()

	// The whole point of this package: dots in labels survive intact.
	p := modulepath.Root().
		Append(modulepath.NewStep("vpc.primary")).
		Append(modulepath.NewStep("inner"))
	assert.Equal(t, "vpc.primary.inner", p.LogicalName())
}

func TestPath_LogicalName_MixedExpansions(t *testing.T) {
	t.Parallel()

	p := modulepath.Root().
		Append(modulepath.NewIndexedStep("outer", 2)).
		Append(modulepath.NewKeyedStep("inner", "prod"))
	assert.Equal(t, `outer[2].inner["prod"]`, p.LogicalName())
}

func TestPath_Parent(t *testing.T) {
	t.Parallel()

	root := modulepath.Root()
	_, _, ok := root.Parent()
	assert.False(t, ok)

	a := root.Append(modulepath.NewStep("a"))
	parent, last, ok := a.Parent()
	assert.True(t, ok)
	assert.True(t, parent.IsRoot())
	assert.Equal(t, "a", last.Name())

	ab := a.Append(modulepath.NewIndexedStep("b", 5))
	parent, last, ok = ab.Parent()
	assert.True(t, ok)
	assert.Equal(t, "a", parent.LogicalName())
	assert.Equal(t, "b[5]", last.LogicalName())
}

func TestPath_Steps(t *testing.T) {
	t.Parallel()

	p := modulepath.Root().
		Append(modulepath.NewStep("a")).
		Append(modulepath.NewIndexedStep("b", 7)).
		Append(modulepath.NewKeyedStep("c", "k"))

	var got []string
	for s := range p.Steps {
		got = append(got, s.LogicalName())
	}
	assert.Equal(t, []string{"a", "b[7]", `c["k"]`}, got)
}

func TestPath_Steps_EarlyExit(t *testing.T) {
	t.Parallel()

	p := modulepath.Root().
		Append(modulepath.NewStep("a")).
		Append(modulepath.NewStep("b")).
		Append(modulepath.NewStep("c"))

	var got []string
	for s := range p.Steps {
		got = append(got, s.Name())
		if s.Name() == "b" {
			break
		}
	}
	assert.Equal(t, []string{"a", "b"}, got)
}

func TestPath_Comparable(t *testing.T) {
	t.Parallel()

	p1 := modulepath.Root().Append(modulepath.NewStep("a"))
	p2 := modulepath.Root().Append(modulepath.NewStep("a"))
	p3 := modulepath.Root().Append(modulepath.NewStep("b"))

	assert.True(t, p1 == p2, "equal paths must be == comparable")
	assert.False(t, p1 == p3)
}

func TestPath_UsableAsMapKey(t *testing.T) {
	t.Parallel()

	p1 := modulepath.Root().Append(modulepath.NewStep("a"))
	p2 := modulepath.Root().Append(modulepath.NewStep("a"))
	p3 := modulepath.Root().Append(modulepath.NewStep("b"))

	m := map[modulepath.Path]string{p1: "first"}
	got, ok := m[p2]
	assert.True(t, ok)
	assert.Equal(t, "first", got)
	_, ok = m[p3]
	assert.False(t, ok)
}

func TestPath_NoCollisionAcrossDottedLabels(t *testing.T) {
	t.Parallel()

	// "a.b" / "c" must not collide with "a" / "b.c" — the bug we set out
	// to fix. With length-prefixed encoding, these produce distinct paths.
	// (Their logical names DO collide — labels containing "." cannot be
	// written in HCL, so LogicalName documents this as a one-way
	// derivation — but the paths themselves stay distinct.)
	p1 := modulepath.Root().
		Append(modulepath.NewStep("a.b")).
		Append(modulepath.NewStep("c"))
	p2 := modulepath.Root().
		Append(modulepath.NewStep("a")).
		Append(modulepath.NewStep("b.c"))

	assert.False(t, p1 == p2)
	assert.Equal(t, "a.b.c", p1.LogicalName())
	assert.Equal(t, "a.b.c", p2.LogicalName())
}

func TestPath_LogicalName_NoCollisionAcrossDashedKeys(t *testing.T) {
	t.Parallel()

	// "-" is legal both in HCL labels and in for_each keys, so bracket
	// quoting must keep a dash in a key from colliding with a dash in a
	// label.
	byKey := modulepath.Root().Append(modulepath.NewKeyedStep("m", "a-b"))
	byLabel := modulepath.Root().Append(modulepath.NewKeyedStep("m-a", "b"))
	assert.Equal(t, `m["a-b"]`, byKey.LogicalName())
	assert.Equal(t, `m-a["b"]`, byLabel.LogicalName())
}

func TestPath_PrefixString(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", modulepath.Root().PrefixString())

	p := modulepath.Root().Append(modulepath.NewStep("a"))
	got := p.PrefixString()
	// Must end with "." so concatenation works.
	assert.True(t, strings.HasSuffix(got, "."))

	// Must contain the label content for diagnostics, but the exact
	// format is part of the API contract: it's a quoted bracketed form.
	assert.Contains(t, got, "a")
}

func TestPath_PrefixString_DistinguishesDottedLabels(t *testing.T) {
	t.Parallel()

	// "a.b" / "c" must produce a different prefix than "a" / "b.c".
	p1 := modulepath.Root().
		Append(modulepath.NewStep("a.b")).
		Append(modulepath.NewStep("c"))
	p2 := modulepath.Root().
		Append(modulepath.NewStep("a")).
		Append(modulepath.NewStep("b.c"))
	assert.NotEqual(t, p1.PrefixString(), p2.PrefixString())
}

func TestPath_String_Diagnostic(t *testing.T) {
	t.Parallel()

	p := modulepath.Root().
		Append(modulepath.NewStep("vpc.primary")).
		Append(modulepath.NewIndexedStep("inner", 3))
	// String is for humans only — exact format may evolve, but it should
	// contain both labels and the index.
	got := p.String()
	assert.Contains(t, got, "vpc.primary")
	assert.Contains(t, got, "inner")
	assert.Contains(t, got, "3")
}

func TestPath_LargeIndex(t *testing.T) {
	t.Parallel()

	// Indices may exceed 64; the encoding allocates a full uint64 so this is fine.
	p := modulepath.Root().Append(modulepath.NewIndexedStep("a", 1<<20))
	assert.Equal(t, "a[1048576]", first(p.Steps).LogicalName())
}

// first returns the first value yielded by an iter.Seq[T]-like function.
// Used to keep tests succinct.
func first[T any](seq func(yield func(T) bool)) T {
	var out T
	for v := range seq {
		out = v
		break
	}
	return out
}

func TestPath_StepsAreOrderPreserving(t *testing.T) {
	t.Parallel()

	steps := []modulepath.Step{
		modulepath.NewStep("a"),
		modulepath.NewIndexedStep("b", 1),
		modulepath.NewKeyedStep("c", "x"),
		modulepath.NewStep("d"),
	}
	p := modulepath.Root()
	for _, s := range steps {
		p = p.Append(s)
	}

	var got []string
	for s := range p.Steps {
		got = append(got, s.LogicalName())
	}
	want := make([]string, 0, len(steps))
	for _, s := range steps {
		want = append(want, s.LogicalName())
	}
	assert.True(t, slices.Equal(want, got), "expected %v, got %v", want, got)
}
