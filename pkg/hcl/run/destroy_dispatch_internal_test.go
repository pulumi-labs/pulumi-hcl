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

package run

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestParseLogicalName(t *testing.T) {
	t.Parallel()

	ptr := func(v int) *int { return &v }
	sptr := func(v string) *string { return &v }

	tests := []struct {
		name string
		want []logicalSeg
	}{
		{name: "target", want: []logicalSeg{{name: "target"}}},
		{name: "target[3]", want: []logicalSeg{{name: "target", index: ptr(3)}}},
		{name: `target["key"]`, want: []logicalSeg{{name: "target", key: sptr("key")}}},
		{name: `m["a.b"].target`, want: []logicalSeg{
			{name: "m", key: sptr("a.b")},
			{name: "target"},
		}},
		{name: `m[0].n["x\"y"].target[1]`, want: []logicalSeg{
			{name: "m", index: ptr(0)},
			{name: "n", key: sptr(`x"y`)},
			{name: "target", index: ptr(1)},
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := parseLogicalName(tt.name)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
			assert.Equal(t, tt.name, renderSegs(got))
		})
	}

	for _, malformed := range []string{"", ".", "a.", ".a", "a[", "a[]", "a[1", `a["x]`, "a[1]b", "a[x]"} {
		t.Run("malformed_"+malformed, func(t *testing.T) {
			t.Parallel()
			_, err := parseLogicalName(malformed)
			assert.Error(t, err)
		})
	}
}

func TestURNChainHelpers(t *testing.T) {
	t.Parallel()

	u := "urn:pulumi:test::proj::hcl:index:Module$simple:index:Resource::comp-target"
	assert.Equal(t, "simple:index:Resource", urnType(u))
	assert.Equal(t, "hcl:index:Module$", urnParentChain(u))

	root := "urn:pulumi:test::proj::simple:index:Resource::target"
	assert.Equal(t, "simple:index:Resource", urnType(root))
	assert.Equal(t, "", urnParentChain(root))
}
