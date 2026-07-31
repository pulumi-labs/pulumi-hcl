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

// Package docs provides language-specific helpers for rendering HCL in
// Pulumi schema-driven documentation.
package docs

import (
	"fmt"
	"strings"

	"github.com/pulumi/pulumi-hcl/pkg/hcl/packages"
	"github.com/pulumi/pulumi-hcl/pkg/hcl/transform"
	"github.com/pulumi/pulumi/pkg/v3/codegen/schema"
)

// DocLanguageHelper is the HCL-specific implementation of [codegen.DocLanguageHelper].
//
// It mirrors the naming conventions used by GenerateProgram: resource and
// invoke tokens are flattened to their HCL form (e.g. "aws_s3_bucket"), and
// property names are emitted in snake_case.
type DocLanguageHelper struct{}

func (d DocLanguageHelper) GetPropertyName(p *schema.Property) (string, error) {
	return transform.SnakeCaseFromPulumiCase(p.Name), nil
}

// HCL has no enum types; render the value itself.
func (d DocLanguageHelper) GetEnumName(e *schema.Enum, typeName string) (string, error) {
	return fmt.Sprintf("%q", e.Value), nil
}

// HCL has no methods.
func (d DocLanguageHelper) GetMethodResultName(schema.PackageReference, string, *schema.Resource, *schema.Method) string {
	// TODO: This is call blocks, HCL does have methods, but not result types.
	return ""
}

func (d DocLanguageHelper) GetMethodName(m *schema.Method) string {
	// TODO: This is call blocks, HCL does have methods.
	return ""
}

// ResolveDocRef always reports the ref as unresolved so callers fall back to
// their default rendering; HCL-specific resolution is not implemented.
func (d DocLanguageHelper) ResolveDocRef(schema.PackageReference, schema.DocRef, schema.DocRef) (string, bool, error) {
	return "", false, nil
}

func (d DocLanguageHelper) GetModuleName(_ schema.PackageReference, modName string) string {
	if modName == "index" {
		return ""
	}
	return modName
}

func (d DocLanguageHelper) GetTypeName(pkg schema.PackageReference, t schema.Type, input bool, relativeTo string) string {
	getType := func(t schema.Type) string {
		return d.GetTypeName(pkg, t, input, relativeTo)
	}
	if schema.IsPrimitiveType(t) {
		switch t {
		case schema.NumberType, schema.IntType:
			return "number"
		case schema.StringType:
			return "string"
		case schema.BoolType:
			return "bool"
		case schema.ArchiveType:
			return "archive"
		case schema.AssetType:
			return "asset"
		case schema.JSONType, schema.AnyType:
			return "any"
		}
	}
	switch t := t.(type) {
	case *schema.ResourceType:
		return tokenToHCLName(pkg, t.Token)
	case *schema.ArrayType:
		return fmt.Sprintf("list(%s)", getType(t.ElementType))
	case *schema.InputType:
		return getType(t.ElementType)
	case *schema.MapType:
		return fmt.Sprintf("map(%s)", getType(t.ElementType))
	case *schema.UnionType:
		if len(t.ElementTypes) == 0 {
			return ""
		}
		var types strings.Builder
		types.WriteString(getType(t.ElementTypes[0]))
		for i := 1; i < len(t.ElementTypes); i++ {
			types.WriteString(" | " + getType(t.ElementTypes[i]))
		}
		return types.String()
	case *schema.EnumType:
		if len(t.Elements) == 0 {
			return ""
		}
		toString := func(v any) string {
			switch v := v.(type) {
			case string:
				return fmt.Sprintf("%q", v)
			default:
				return fmt.Sprintf("%v", v)
			}
		}
		var values strings.Builder
		values.WriteString(toString(t.Elements[0].Value))
		for i := 1; i < len(t.Elements); i++ {
			values.WriteString(" | " + toString(t.Elements[i].Value))
		}
		return values.String()
	case *schema.OptionalType:
		return getType(t.ElementType)
	case *schema.ObjectType:
		return "object"
	default:
		return ""
	}
}

func (d DocLanguageHelper) GetFunctionName(f *schema.Function) string {
	return tokenToHCLName(f.PackageReference, f.Token)
}

func (d DocLanguageHelper) GetResourceName(r *schema.Resource) string {
	return tokenToHCLName(r.PackageReference, r.Token)
}

// tokenToHCLName converts a Pulumi token to its HCL form, normalizing the module
// component using the package's TokenToModule mapping. The schema token may
// include a sub-module (e.g. "random:index/randomString:RandomString") that the
// HCL emitter strips during program generation; routing through TokenToModule
// matches that behavior so doc names line up with generated examples.
func tokenToHCLName(pkg schema.PackageReference, token string) string {
	hclName, _ := packages.PulumiResourceTokenToHCL(pkg, token)
	return hclName
}

func (d DocLanguageHelper) GetResourceFunctionResultName(modName string, f *schema.Function) string {
	return ""
}

// Doc links

func (d DocLanguageHelper) GetDocLinkForResourceType(pkg *schema.Package, moduleName, typeName string) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForPulumiType(pkg *schema.Package, typeName string) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForResourceInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return ""
}

func (d DocLanguageHelper) GetDocLinkForFunctionInputOrOutputType(pkg *schema.Package, moduleName, typeName string, input bool) string {
	return ""
}
