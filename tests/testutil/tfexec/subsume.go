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

package tfexec

import (
	"fmt"
	"reflect"
	"strings"
)

// Subsumes checks the cross-runtime operation relationship: pul must be an
// order-preserving subsequence of tf, and every tf op left unmatched must be
// an exact duplicate of some pul op. In words: OpenTofu must cause exactly
// what Pulumi causes, in the same order, plus re-evaluations of calls it
// already made.
//
// The leftovers exist because one `tofu apply` runs several expression-
// evaluation walks (validate, plan, apply) and re-evaluates a provider
// function call in every walk where its arguments are already known, while
// the Pulumi path evaluates the program once per operation. A call whose
// arguments depend on another op's result is only evaluable in the final
// walk, so it records exactly once on both sides — which is also why the
// relationship is a subsequence and not a suffix: a data-source read with
// static config records only in tofu's plan walk, ahead of function-call
// re-records that never precede it on the Pulumi side.
//
// Which unmatched op counts as "leftover" does not depend on how the
// subsequence match is chosen: every matched op equals its pul counterpart,
// so the unmatched multiset is always tf minus pul, and the duplicate check
// is equivalent to requiring each of those ops to appear somewhere in pul.
func Subsumes(tf, pul []Op) error {
	matched := make([]bool, len(tf))
	ti := 0
	for pi := range pul {
		found := false
		for ; ti < len(tf); ti++ {
			if reflect.DeepEqual(tf[ti], pul[pi]) {
				matched[ti] = true
				ti++
				found = true
				break
			}
		}
		if !found {
			return fmt.Errorf(
				"pulumi op %d has no match in the remaining tofu ops:\n  %s\n%s",
				pi, formatOp(pul[pi]), formatSides(tf, pul))
		}
	}

	for i, op := range tf {
		if matched[i] {
			continue
		}
		dup := false
		for pi := range pul {
			if reflect.DeepEqual(op, pul[pi]) {
				dup = true
				break
			}
		}
		if !dup {
			return fmt.Errorf(
				"tofu op %d is not a duplicate of any pulumi op:\n  %s\n%s",
				i, formatOp(op), formatSides(tf, pul))
		}
	}
	return nil
}

func formatOp(op Op) string {
	return fmt.Sprintf("kind=%d type=%s inputs=%v outputs=%v", op.Kind, op.Type, op.Inputs, op.Outputs)
}

func formatSides(tf, pul []Op) string {
	var b strings.Builder
	fmt.Fprintf(&b, "tofu ops (%d):\n", len(tf))
	for i, op := range tf {
		fmt.Fprintf(&b, "  %2d. %s\n", i, formatOp(op))
	}
	fmt.Fprintf(&b, "pulumi ops (%d):\n", len(pul))
	for i, op := range pul {
		fmt.Fprintf(&b, "  %2d. %s\n", i, formatOp(op))
	}
	return b.String()
}
