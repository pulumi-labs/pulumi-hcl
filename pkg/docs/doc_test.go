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

package docs

import (
	"context"
	"errors"
	"testing"

	"github.com/blang/semver"
	"github.com/pulumi/pulumi/pkg/v3/codegen"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

var helper codegen.DocLanguageHelper = DocLanguageHelper{}

func TestGetPropertyName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		input    string
		expected string
	}{
		{"fooBar", "foo_bar"},
		{"FooBar", "foo_bar"},
		{"foo", "foo"},
		{"foo_bar", "foo_bar"},
		{"HTTPServer", "http_server"},
		{"parseHTTPRequest", "parse_http_request"},
	}

	for _, c := range cases {
		t.Run(c.input, func(t *testing.T) {
			t.Parallel()
			got, err := helper.GetPropertyName(&schema.Property{Name: c.input})
			require.NoError(t, err)
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestGetEnumName(t *testing.T) {
	t.Parallel()

	got, err := helper.GetEnumName(&schema.Enum{Value: "Active"}, "Status")
	require.NoError(t, err)
	assert.Equal(t, `"Active"`, got)

	got, err = helper.GetEnumName(&schema.Enum{Value: 42}, "Code")
	require.NoError(t, err)
	assert.Equal(t, `'*'`, got)
}

func TestGetModuleName(t *testing.T) {
	t.Parallel()

	assert.Equal(t, "", helper.GetModuleName(nil, "index"))
	assert.Equal(t, "s3", helper.GetModuleName(nil, "s3"))
	assert.Equal(t, "ec2/instance", helper.GetModuleName(nil, "ec2/instance"))
}

func TestGetTypeName_Primitives(t *testing.T) {
	t.Parallel()

	cases := []struct {
		typ      schema.Type
		expected string
	}{
		{schema.NumberType, "number"},
		{schema.IntType, "number"},
		{schema.StringType, "string"},
		{schema.BoolType, "bool"},
		{schema.ArchiveType, "archive"},
		{schema.AssetType, "asset"},
		{schema.JSONType, "any"},
		{schema.AnyType, "any"},
	}

	for _, c := range cases {
		t.Run(c.typ.String(), func(t *testing.T) {
			t.Parallel()
			assert.Equal(t, c.expected, helper.GetTypeName(nil, c.typ, false, ""))
		})
	}
}

func TestGetTypeName_Composite(t *testing.T) {
	t.Parallel()

	t.Run("array of strings", func(t *testing.T) {
		t.Parallel()
		typ := &schema.ArrayType{ElementType: schema.StringType}
		assert.Equal(t, "list(string)", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("map of numbers", func(t *testing.T) {
		t.Parallel()
		typ := &schema.MapType{ElementType: schema.NumberType}
		assert.Equal(t, "map(number)", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("nested array of map", func(t *testing.T) {
		t.Parallel()
		typ := &schema.ArrayType{ElementType: &schema.MapType{ElementType: schema.BoolType}}
		assert.Equal(t, "list(map(bool))", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("input type unwraps", func(t *testing.T) {
		t.Parallel()
		typ := &schema.InputType{ElementType: schema.StringType}
		assert.Equal(t, "string", helper.GetTypeName(nil, typ, true, ""))
	})

	t.Run("optional unwraps", func(t *testing.T) {
		t.Parallel()
		typ := &schema.OptionalType{ElementType: schema.IntType}
		assert.Equal(t, "number", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("object", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "object", helper.GetTypeName(nil, &schema.ObjectType{}, false, ""))
	})

	t.Run("union", func(t *testing.T) {
		t.Parallel()
		typ := &schema.UnionType{ElementTypes: []schema.Type{schema.StringType, schema.NumberType, schema.BoolType}}
		assert.Equal(t, "string | number | bool", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("empty union", func(t *testing.T) {
		t.Parallel()
		typ := &schema.UnionType{}
		assert.Equal(t, "", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("enum of strings", func(t *testing.T) {
		t.Parallel()
		typ := &schema.EnumType{Elements: []*schema.Enum{{Value: "a"}, {Value: "b"}}}
		assert.Equal(t, `"a" | "b"`, helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("enum of numbers", func(t *testing.T) {
		t.Parallel()
		typ := &schema.EnumType{Elements: []*schema.Enum{{Value: 1}, {Value: 2}}}
		assert.Equal(t, "1 | 2", helper.GetTypeName(nil, typ, false, ""))
	})

	t.Run("empty enum", func(t *testing.T) {
		t.Parallel()
		assert.Equal(t, "", helper.GetTypeName(nil, &schema.EnumType{}, false, ""))
	})

	t.Run("resource type uses HCL token", func(t *testing.T) {
		t.Parallel()
		typ := &schema.ResourceType{Token: "aws:s3:Bucket"}
		assert.Equal(t, "aws_s3_bucket", helper.GetTypeName(nil, typ, false, ""))
	})
}

func TestGetResourceName(t *testing.T) {
	t.Parallel()

	cases := []struct {
		token    string
		expected string
	}{
		{"aws:s3:Bucket", "aws_s3_bucket"},
		{"aws:index:Provider", "aws_provider"},
		{"pulumi:pulumi:StackReference", "pulumi_stack_reference"},
	}
	for _, c := range cases {
		t.Run(c.token, func(t *testing.T) {
			t.Parallel()
			got := helper.GetResourceName(&schema.Resource{Token: c.token})
			assert.Equal(t, c.expected, got)
		})
	}
}

func TestGetFunctionName(t *testing.T) {
	t.Parallel()

	got := helper.GetFunctionName(&schema.Function{Token: "aws:s3:getBucket"})
	assert.Equal(t, "aws_s3_get_bucket", got)
}

// TestTokenNormalization_ViaPackage verifies that names are normalized through
// the package's TokenToModule mapping. Schemas that use a moduleFormat regex
// (such as pulumi-random's "(.*)(?:/[^/]*)") emit tokens like
// "random:index/randomString:RandomString" but expect the HCL form to collapse
// to "random_random_string" — matching what GenerateProgram actually emits.
func TestTokenNormalization_ViaPackage(t *testing.T) {
	t.Parallel()

	pkg := bindTestPackage(t, schema.PackageSpec{
		Name: "random",
		Meta: &schema.MetadataSpec{
			ModuleFormat: "(.*)(?:/[^/]*)",
		},
		Resources: map[string]schema.ResourceSpec{
			"random:index/randomString:RandomString": {},
		},
		Functions: map[string]schema.FunctionSpec{
			"random:index/getRandom:getRandom": {},
		},
	})

	require.Equal(t, "", pkg.TokenToModule("random:index/randomString:RandomString"),
		"sanity check: moduleFormat collapses the submodule and the schema package "+
			"normalizes 'index' to empty")

	t.Run("resource", func(t *testing.T) {
		t.Parallel()
		r, ok, err := pkg.Resources().Get("random:index/randomString:RandomString")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "random_random_string", helper.GetResourceName(r))
		typ := &schema.ResourceType{Token: r.Token, Resource: r}
		assert.Equal(t, "random_random_string", helper.GetTypeName(pkg, typ, false, ""))
	})

	t.Run("function", func(t *testing.T) {
		t.Parallel()
		f, ok, err := pkg.Functions().Get("random:index/getRandom:getRandom")
		require.NoError(t, err)
		require.True(t, ok)
		assert.Equal(t, "random_get_random", helper.GetFunctionName(f))
	})
}

// TestGetTypeName_ResourceWithoutPackage falls back to the raw PulumiResourceTokenToHCL
// when no package is provided to resolve the module via TokenToModule.
func TestGetTypeName_ResourceWithoutPackage(t *testing.T) {
	t.Parallel()

	typ := &schema.ResourceType{Token: "random:index/randomString:RandomString"}
	// Without a package to consult TokenToModule, the raw token's submodule is
	// preserved.
	assert.Equal(t, "random_index_random_string_random_string",
		helper.GetTypeName(nil, typ, false, ""))
}

type nopLoader struct{}

func (nopLoader) LoadPackage(string, *semver.Version) (*schema.Package, error) {
	return nil, errNotFound
}

func (nopLoader) LoadPackageReference(string, *semver.Version) (schema.PackageReference, error) {
	return nil, errNotFound
}

func (nopLoader) LoadPackageV2(context.Context, *schema.PackageDescriptor) (*schema.Package, error) {
	return nil, errNotFound
}

func (nopLoader) LoadPackageReferenceV2(context.Context, *schema.PackageDescriptor) (schema.PackageReference, error) {
	return nil, errNotFound
}

var errNotFound = errors.New("package not found")

func bindTestPackage(t *testing.T, spec schema.PackageSpec) schema.PackageReference {
	t.Helper()
	pkg, diags, err := schema.BindSpec(spec, nopLoader{}, schema.ValidationOptions{})
	require.NoError(t, err)
	require.Empty(t, diags)
	return pkg.Reference()
}

