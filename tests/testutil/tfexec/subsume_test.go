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

package tfexec_test

import (
	"testing"

	"github.com/pulumi-labs/pulumi-hcl/tests/testutil/tfexec"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Shorthand op constructors for Subsumes tests.
func callOp(args ...any) tfexec.Op {
	return tfexec.Op{
		Kind:    tfexec.OpCallFunction,
		Type:    "concat_str",
		Inputs:  map[string]any{"args": args},
		Outputs: map[string]any{"result": "joined"},
	}
}

func createOp(name string) tfexec.Op {
	return tfexec.Op{
		Kind:    tfexec.OpCreate,
		Type:    "pfx_thing",
		Inputs:  map[string]any{"name": name},
		Outputs: map[string]any{"name": name, "id": "pfx-id"},
	}
}

func readOp() tfexec.Op {
	return tfexec.Op{
		Kind:    tfexec.OpDataSource,
		Type:    "pfx_lookup",
		Inputs:  map[string]any{"value": nil},
		Outputs: map[string]any{"value": "pfx-value"},
	}
}

func TestSubsumes(t *testing.T) {
	t.Parallel()

	fa, fb := callOp("a-", "1"), callOp("b-", "pfx-id")
	create := createOp("a-1")
	read := readOp()

	t.Run("identical slices", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(
			[]tfexec.Op{fa, create, fb},
			[]tfexec.Op{fa, create, fb}))
	})

	t.Run("empty slices", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(nil, nil))
	})

	// The canonical probe: function(a) -> resource -> function(b) -> output.
	// Tofu re-evaluates the statically-known function(a) in every walk (4
	// records); the chain-dependent function(b) records once on both sides.
	t.Run("re-evaluated function call before the chain", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(
			[]tfexec.Op{fa, fa, fa, fa, create, fb},
			[]tfexec.Op{fa, create, fb}))
	})

	// The data-source probe — the shape that ruled out a plain suffix
	// relationship: tofu reads a static-config data source in the plan walk
	// only, so the read sits in the prefix with re-evaluated function calls
	// after it.
	t.Run("plan-only data read upstream of re-evaluated calls", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(
			[]tfexec.Op{read, fa, fa, fa, create, fb},
			[]tfexec.Op{read, fa, create, fb}))

		// The same tofu recording is NOT a suffix match: the pulumi slice
		// must be found as a subsequence, not at the tail.
		assert.NotEqual(t,
			[]tfexec.Op{fa, fa, create, fb},
			[]tfexec.Op{read, fa, create, fb},
			"sanity: the tail really differs from the pulumi slice")
	})

	// A tofu re-evaluation may also land after the match (e.g. an output
	// expression re-evaluated at the end of the apply walk).
	t.Run("duplicate leftover after the match", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(
			[]tfexec.Op{fa, create, fa},
			[]tfexec.Op{fa, create}))
	})

	t.Run("pulumi op missing from tofu", func(t *testing.T) {
		t.Parallel()
		err := tfexec.Subsumes(
			[]tfexec.Op{fa, create},
			[]tfexec.Op{fa, create, fb})
		require.ErrorContains(t, err, "pulumi op 2 has no match")
	})

	// Order violation: both sides recorded the same ops, but pulumi's create
	// arrived before the call tofu sequenced first. Ordered input must fail.
	t.Run("order flip is not a subsequence", func(t *testing.T) {
		t.Parallel()
		err := tfexec.Subsumes(
			[]tfexec.Op{fa, create},
			[]tfexec.Op{create, fa})
		require.ErrorContains(t, err, "has no match")
	})

	// A tofu op that pulumi never caused is not a legitimate leftover, even
	// though the subsequence match itself succeeds.
	t.Run("leftover that duplicates nothing", func(t *testing.T) {
		t.Parallel()
		err := tfexec.Subsumes(
			[]tfexec.Op{fa, fb, create},
			[]tfexec.Op{fa, create})
		require.ErrorContains(t, err, "tofu op 1 is not a duplicate of any pulumi op")
	})

	// If pulumi records nothing, tofu must record nothing: there is no
	// matched op for a leftover to duplicate.
	t.Run("tofu-only activity with empty pulumi", func(t *testing.T) {
		t.Parallel()
		err := tfexec.Subsumes([]tfexec.Op{fa}, nil)
		require.ErrorContains(t, err, "tofu op 0 is not a duplicate of any pulumi op")
	})

	// Pulumi may legitimately record duplicates too (one read per stage);
	// tofu must then supply a matching occurrence for each.
	t.Run("pulumi-side duplicates need matching tofu occurrences", func(t *testing.T) {
		t.Parallel()
		require.NoError(t, tfexec.Subsumes(
			[]tfexec.Op{read, read, create},
			[]tfexec.Op{read, read, create}))
		err := tfexec.Subsumes(
			[]tfexec.Op{read, create},
			[]tfexec.Op{read, read, create})
		require.ErrorContains(t, err, "pulumi op 1 has no match")
	})

	// Ops that differ only in outputs are distinct: a duplicate must match
	// exactly, and a divergent output is never subsumed.
	t.Run("outputs are part of op identity", func(t *testing.T) {
		t.Parallel()
		divergent := fa
		divergent.Outputs = map[string]any{"result": "different"}
		err := tfexec.Subsumes(
			[]tfexec.Op{divergent, fa, create},
			[]tfexec.Op{fa, create})
		require.ErrorContains(t, err, "tofu op 0 is not a duplicate of any pulumi op")
	})
}
