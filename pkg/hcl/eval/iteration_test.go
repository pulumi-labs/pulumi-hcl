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
	"testing"

	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// countIndexOf reads count.index out of a context's HCL variable table, the way
// a resource's config expression (`element(x, count.index)`) resolves it.
func countIndexOf(t *testing.T, c *Context) int64 {
	t.Helper()
	v, ok := c.HCLContext().Variables["count"]
	require.Truef(t, ok && !v.IsNull(), "count is unbound")
	i, _ := v.GetAttr("index").AsBigFloat().Int64()
	return i
}

// TestWithIterationIsolatesConcurrentIterations pins the property that fixes the
// shared-context count race: two registrations that run concurrently against the
// same base context must each observe their own count.index, never the other's.
//
// The channels force exactly the interleaving that corrupts a shared binding —
// goroutine A takes its view, then B takes and reads its own view (a different
// index), and only then does A read. If the iteration were stored on the shared
// base context (the old SetCount/ClearCount approach), A would read B's index
// here. The gate makes that failure deterministic rather than timing-dependent;
// no sleeps involved.
func TestWithIterationIsolatesConcurrentIterations(t *testing.T) {
	t.Parallel()

	base, err := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	require.NoError(t, err)

	aTookView := make(chan struct{})
	bRead := make(chan struct{})
	aFinished := make(chan struct{})

	var aIndex int64
	go func() {
		defer close(aFinished)
		idx := 3
		a := base.WithIteration(&idx, nil, nil)
		close(aTookView)
		<-bRead
		aIndex = countIndexOf(t, a)
	}()

	<-aTookView
	idx := 7
	b := base.WithIteration(&idx, nil, nil)
	bIndex := countIndexOf(t, b)
	close(bRead)
	<-aFinished

	require.Equal(t, int64(3), aIndex, "A observed B's count.index — the iteration binding is shared, not per-view")
	require.Equal(t, int64(7), bIndex)
}

// TestWithIterationIsolatesConcurrentForEach is the each.key/each.value analogue
// of TestWithIterationIsolatesConcurrentIterations, guarding the same fix on the
// for_each path.
func TestWithIterationIsolatesConcurrentForEach(t *testing.T) {
	t.Parallel()

	base, err := NewContext("/tmp", "/tmp", "/tmp", "", "", "")
	require.NoError(t, err)

	eachKeyOf := func(c *Context) string {
		v, ok := c.HCLContext().Variables["each"]
		require.Truef(t, ok && !v.IsNull(), "each is unbound")
		return v.GetAttr("key").AsString()
	}

	aTookView := make(chan struct{})
	bRead := make(chan struct{})
	aFinished := make(chan struct{})

	var aKey string
	go func() {
		defer close(aFinished)
		key, val := cty.StringVal("a"), cty.StringVal("va")
		a := base.WithIteration(nil, &key, &val)
		close(aTookView)
		<-bRead
		aKey = eachKeyOf(a)
	}()

	<-aTookView
	key, val := cty.StringVal("b"), cty.StringVal("vb")
	b := base.WithIteration(nil, &key, &val)
	bKey := eachKeyOf(b)
	close(bRead)
	<-aFinished

	require.Equal(t, "a", aKey, "A observed B's each.key — the iteration binding is shared, not per-view")
	require.Equal(t, "b", bKey)
}
