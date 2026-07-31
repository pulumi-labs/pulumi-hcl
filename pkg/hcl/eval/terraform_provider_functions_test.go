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

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// Expected strings match OpenTofu v1.12.3's encode_expr output byte for byte,
// including hclwrite's attribute alignment and nested-object indentation.
func TestEncodeExpr(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		arg  cty.Value
		want cty.Value
	}{
		{
			name: "list",
			arg:  cty.TupleVal([]cty.Value{cty.NumberIntVal(1), cty.NumberIntVal(2)}),
			want: cty.StringVal("[1, 2]"),
		},
		{
			name: "string",
			arg:  cty.StringVal("hello"),
			want: cty.StringVal(`"hello"`),
		},
		{
			name: "object",
			arg: cty.ObjectVal(map[string]cty.Value{
				"name": cty.StringVal("alice"),
				"tags": cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
				"n":    cty.NumberIntVal(3),
			}),
			want: cty.StringVal("{\n  n    = 3\n  name = \"alice\"\n  tags = [\"a\", \"b\"]\n}"),
		},
		{
			name: "nested",
			arg: cty.TupleVal([]cty.Value{
				cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1)}),
				cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(2)}),
			}),
			want: cty.StringVal("[{\n  a = 1\n  }, {\n  a = 2\n}]"),
		},
		{
			name: "wholly unknown defers",
			arg:  cty.UnknownVal(cty.String),
			want: cty.UnknownVal(cty.String),
		},
		{
			name: "sensitive mark passes through",
			arg:  cty.StringVal("secret").Mark(SensitiveMark),
			want: cty.StringVal(`"secret"`).Mark(SensitiveMark),
		},
		{
			name: "nested sensitive mark passes through",
			arg: cty.ObjectVal(map[string]cty.Value{
				"pw":    cty.StringVal("secret").Mark(SensitiveMark),
				"plain": cty.NumberIntVal(1),
			}),
			want: cty.StringVal("{\n  plain = 1\n  pw    = \"secret\"\n}").Mark(SensitiveMark),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := encodeExprFunc.Call([]cty.Value{tt.arg})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}

	errTests := []struct {
		name    string
		arg     cty.Value
		wantErr string
	}{
		{
			name:    "null",
			arg:     cty.NullVal(cty.DynamicPseudoType),
			wantErr: "argument must not be null",
		},
		{
			name: "partially unknown",
			arg: cty.ObjectVal(map[string]cty.Value{
				"known":   cty.NumberIntVal(1),
				"unknown": cty.UnknownVal(cty.String),
			}),
			wantErr: "input is not wholly known",
		},
	}
	for _, tt := range errTests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := encodeExprFunc.Call([]cty.Value{tt.arg})
			assert.ErrorContains(t, err, tt.wantErr)
		})
	}
}
