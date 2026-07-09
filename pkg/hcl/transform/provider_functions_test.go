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

package transform

import (
	"errors"
	"testing"

	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
)

// concatFunction is a two-string-argument function with an optional second
// argument and a scalar string return, the shape the bridge generates for
// `concat(a string, b string|null) string`.
func concatFunction() *schema.Function {
	return &schema.Function{
		Token:               "simple:index/concatStr:concatStr",
		MultiArgumentInputs: true,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "first", Type: schema.StringType},
				{Name: "second", Type: &schema.OptionalType{ElementType: schema.StringType}},
			},
		},
		ReturnType: schema.StringType,
	}
}

func TestProviderFunctionInvokesWithPositionalArgs(t *testing.T) {
	t.Parallel()

	var got property.Map
	f, err := ProviderFunction(concatFunction(), false, false, func(args property.Map) (property.Map, error) {
		got = args
		return property.NewMap(map[string]property.Value{"result": property.New("a-b")}), nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
	require.NoError(t, err)

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"first":  property.New("a"),
		"second": property.New("b"),
	}), got)
	assert.Equal(t, cty.StringVal("a-b"), v)
}

func TestProviderFunctionOptionalArgumentAcceptsNull(t *testing.T) {
	t.Parallel()

	var got property.Map
	f, err := ProviderFunction(concatFunction(), false, false, func(args property.Map) (property.Map, error) {
		got = args
		return property.NewMap(map[string]property.Value{"result": property.New("a")}), nil
	})
	require.NoError(t, err)

	_, err = f.Call([]cty.Value{cty.StringVal("a"), cty.NullVal(cty.String)})
	require.NoError(t, err)

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"first":  property.New("a"),
		"second": property.New(property.Null),
	}), got)
}

func TestProviderFunctionRequiredArgumentRejectsNull(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, false, func(property.Map) (property.Map, error) {
		t.Fatal("impl must not be called")
		return property.Map{}, nil
	})
	require.NoError(t, err)

	_, err = f.Call([]cty.Value{cty.NullVal(cty.String), cty.StringVal("b")})
	assert.ErrorContains(t, err, "must not be null")
}

func TestProviderFunctionUnknownArgumentSkipsInvoke(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, false, func(property.Map) (property.Map, error) {
		t.Fatal("impl must not be called")
		return property.Map{}, nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.UnknownVal(cty.String).Mark("dep"), cty.StringVal("b")})
	require.NoError(t, err)

	assert.Equal(t, cty.UnknownVal(cty.String).Mark("dep"), v)
}

func TestProviderFunctionMarksTransferToResult(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, false, func(args property.Map) (property.Map, error) {
		// Marks are stripped before conversion, so the impl sees plain values.
		assert.Equal(t, property.NewMap(map[string]property.Value{
			"first":  property.New("a"),
			"second": property.New("b"),
		}), args)
		return property.NewMap(map[string]property.Value{"result": property.New("a-b")}), nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.StringVal("a").Mark("sensitive"), cty.StringVal("b")})
	require.NoError(t, err)

	assert.Equal(t, cty.StringVal("a-b").Mark("sensitive"), v)
}

func TestProviderFunctionEmptyResultDuringPreviewIsUnknown(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, true, func(property.Map) (property.Map, error) {
		return property.Map{}, nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
	require.NoError(t, err)

	assert.Equal(t, cty.UnknownVal(cty.String), v)
}

func TestProviderFunctionImplErrorSurfaces(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, false, func(property.Map) (property.Map, error) {
		return property.Map{}, errors.New("boom")
	})
	require.NoError(t, err)

	_, err = f.Call([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
	assert.ErrorContains(t, err, "boom")
}

func TestProviderFunctionVariadic(t *testing.T) {
	t.Parallel()

	fn := &schema.Function{
		Token:               "simple:index/join:join",
		MultiArgumentInputs: true,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "separator", Type: schema.StringType},
				{Name: "parts", Type: &schema.OptionalType{
					ElementType: &schema.ArrayType{ElementType: schema.StringType},
				}},
			},
		},
		ReturnType: schema.StringType,
	}

	var got property.Map
	f, err := ProviderFunction(fn, true, false, func(args property.Map) (property.Map, error) {
		got = args
		return property.NewMap(map[string]property.Value{"result": property.New("a-b-c")}), nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{
		cty.StringVal("-"), cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
	})
	require.NoError(t, err)

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"separator": property.New("-"),
		"parts": property.New([]property.Value{
			property.New("a"), property.New("b"), property.New("c"),
		}),
	}), got)
	assert.Equal(t, cty.StringVal("a-b-c"), v)
}

func TestProviderFunctionVariadicAcceptsZeroArguments(t *testing.T) {
	t.Parallel()

	fn := &schema.Function{
		Token:               "simple:index/join:join",
		MultiArgumentInputs: true,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "separator", Type: schema.StringType},
				{Name: "parts", Type: &schema.OptionalType{
					ElementType: &schema.ArrayType{ElementType: schema.StringType},
				}},
			},
		},
		ReturnType: schema.StringType,
	}

	var got property.Map
	f, err := ProviderFunction(fn, true, false, func(args property.Map) (property.Map, error) {
		got = args
		return property.NewMap(map[string]property.Value{"result": property.New("")}), nil
	})
	require.NoError(t, err)

	_, err = f.Call([]cty.Value{cty.StringVal("-")})
	require.NoError(t, err)

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"separator": property.New("-"),
		"parts":     property.New([]property.Value{}),
	}), got)
}

func TestProviderFunctionNonVariadicTrailingArrayTakesListArgument(t *testing.T) {
	t.Parallel()

	// The same schema shape without the mapping's variadic marker is a genuine
	// trailing list parameter: callers pass a list value, not spread arguments.
	fn := &schema.Function{
		Token:               "simple:index/join:join",
		MultiArgumentInputs: true,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "separator", Type: schema.StringType},
				{Name: "parts", Type: &schema.OptionalType{
					ElementType: &schema.ArrayType{ElementType: schema.StringType},
				}},
			},
		},
		ReturnType: schema.StringType,
	}

	var got property.Map
	f, err := ProviderFunction(fn, false, false, func(args property.Map) (property.Map, error) {
		got = args
		return property.NewMap(map[string]property.Value{"result": property.New("a-b")}), nil
	})
	require.NoError(t, err)

	_, err = f.Call([]cty.Value{
		cty.StringVal("-"), cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
	})
	require.NoError(t, err)

	assert.Equal(t, property.NewMap(map[string]property.Value{
		"separator": property.New("-"),
		"parts":     property.New([]property.Value{property.New("a"), property.New("b")}),
	}), got)
}

func TestProviderFunctionObjectReturn(t *testing.T) {
	t.Parallel()

	fn := &schema.Function{
		Token:               "simple:index/parseArn:parseArn",
		MultiArgumentInputs: true,
		Inputs: &schema.ObjectType{
			Properties: []*schema.Property{{Name: "arn", Type: schema.StringType}},
		},
		ReturnType: &schema.ObjectType{
			Properties: []*schema.Property{
				{Name: "accountId", Type: schema.StringType},
				{Name: "region", Type: schema.StringType},
			},
		},
	}

	f, err := ProviderFunction(fn, false, false, func(property.Map) (property.Map, error) {
		return property.NewMap(map[string]property.Value{
			"accountId": property.New("123"),
			"region":    property.New("us-west-2"),
		}), nil
	})
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.StringVal("arn:aws:...")})
	require.NoError(t, err)

	assert.Equal(t, cty.ObjectVal(map[string]cty.Value{
		"account_id": cty.StringVal("123"),
		"region":     cty.StringVal("us-west-2"),
	}), v)
}

func TestProviderFunctionRejectsSingleBagInvokes(t *testing.T) {
	t.Parallel()

	fn := concatFunction()
	fn.MultiArgumentInputs = false
	_, err := ProviderFunction(fn, false, false, nil)
	assert.ErrorContains(t, err, "does not take positional arguments")
}

// TestProviderFunctionNilImplReturnsRefinedUnknown covers the type-only
// projection: with no impl, calls evaluate to a refined unknown of the return
// type even when arguments are unknown (which would short-circuit a runtime
// function before its impl runs).
func TestProviderFunctionNilImplReturnsRefinedUnknown(t *testing.T) {
	t.Parallel()

	f, err := ProviderFunction(concatFunction(), false, false, nil)
	require.NoError(t, err)

	v, err := f.Call([]cty.Value{cty.StringVal("a"), cty.StringVal("b")})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.String).RefineNotNull(), v)

	v, err = f.Call([]cty.Value{cty.UnknownVal(cty.String), cty.StringVal("b")})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.String).RefineNotNull(), v)
}
