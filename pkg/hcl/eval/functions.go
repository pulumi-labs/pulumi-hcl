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

// Package eval implements expression evaluation for HCL configurations.
package eval

import (
	"bytes"
	"compress/gzip"
	"crypto/md5"
	"crypto/rand"
	"crypto/rsa"
	"crypto/sha1"
	"crypto/sha256"
	"crypto/sha512"
	"crypto/x509"
	"encoding/base64"
	"encoding/csv"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/apparentlymart/go-cidr/cidr"
	"github.com/google/uuid"
	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/ext/customdecode"
	"github.com/hashicorp/hcl/v2/ext/tryfunc"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/archive"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/asset"
	"github.com/pulumi/pulumi/sdk/v3/go/common/resource/urn"
	"github.com/pulumi/pulumi/sdk/v3/go/property"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/convert"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

var (
	// AssetCapsuleType is the cty capsule type for Pulumi assets.
	AssetCapsuleType = cty.Capsule("Asset", reflect.TypeFor[asset.Asset]())

	// ArchiveCapsuleType is the cty capsule type for Pulumi archives.
	ArchiveCapsuleType = cty.Capsule("Archive", reflect.TypeFor[archive.Archive]())

	// ResourceReferenceCapsuleType is the cty capsule type for Pulumi resource references.
	ResourceReferenceCapsuleType = cty.Capsule("ResourceReference", reflect.TypeFor[property.ResourceReference]())
)

// Functions returns a map of all Terraform-compatible functions.
func Functions(baseDir string) map[string]function.Function {
	funcs := map[string]function.Function{
		// Numeric functions
		"abs":      stdlib.AbsoluteFunc,
		"ceil":     stdlib.CeilFunc,
		"floor":    stdlib.FloorFunc,
		"log":      stdlib.LogFunc,
		"max":      stdlib.MaxFunc,
		"min":      stdlib.MinFunc,
		"pow":      stdlib.PowFunc,
		"signum":   stdlib.SignumFunc,
		"parseint": stdlib.ParseIntFunc,

		// String functions
		"chomp":       stdlib.ChompFunc,
		"format":      stdlib.FormatFunc,
		"formatlist":  stdlib.FormatListFunc,
		"indent":      stdlib.IndentFunc,
		"join":        stdlib.JoinFunc,
		"lower":       stdlib.LowerFunc,
		"regex":       stdlib.RegexFunc,
		"regexall":    stdlib.RegexAllFunc,
		"replace":     stdlib.ReplaceFunc,
		"split":       stdlib.SplitFunc,
		"strrev":      stdlib.ReverseFunc,
		"substr":      stdlib.SubstrFunc,
		"title":       stdlib.TitleFunc,
		"trim":        stdlib.TrimFunc,
		"trimprefix":  stdlib.TrimPrefixFunc,
		"trimsuffix":  stdlib.TrimSuffixFunc,
		"trimspace":   stdlib.TrimSpaceFunc,
		"upper":       stdlib.UpperFunc,
		"startswith":  startsWithFunc,
		"endswith":    endsWithFunc,
		"strcontains": strContainsFunc,

		// Collection functions
		"alltrue":         allTrueFunc,
		"anytrue":         anyTrueFunc,
		"chunklist":       stdlib.ChunklistFunc,
		"coalesce":        coalesceFunc,
		"coalescelist":    stdlib.CoalesceListFunc,
		"compact":         stdlib.CompactFunc,
		"concat":          stdlib.ConcatFunc,
		"contains":        stdlib.ContainsFunc,
		"distinct":        stdlib.DistinctFunc,
		"element":         stdlib.ElementFunc,
		"flatten":         stdlib.FlattenFunc,
		"index":           indexFunc,
		"keys":            stdlib.KeysFunc,
		"length":          lengthFunc,
		"list":            listFunc,
		"lookup":          lookupFunc,
		"map":             mapFunc,
		"matchkeys":       matchkeysFunc,
		"merge":           stdlib.MergeFunc,
		"one":             oneFunc,
		"range":           stdlib.RangeFunc,
		"reverse":         stdlib.ReverseListFunc,
		"setintersection": stdlib.SetIntersectionFunc,
		"setproduct":      stdlib.SetProductFunc,
		"setsubtract":     stdlib.SetSubtractFunc,
		"setunion":        stdlib.SetUnionFunc,
		"slice":           stdlib.SliceFunc,
		"sort":            stdlib.SortFunc,
		"sum":             sumFunc,
		"transpose":       transposeFunc,
		"values":          stdlib.ValuesFunc,
		"zipmap":          stdlib.ZipmapFunc,
		"entries":         entriesFunc,

		// Encoding functions
		"base64decode":     base64DecodeFunc,
		"base64encode":     base64EncodeFunc,
		"base64gzip":       base64GzipFunc,
		"csvdecode":        stdlib.CSVDecodeFunc,
		"jsondecode":       stdlib.JSONDecodeFunc,
		"jsonencode":       stdlib.JSONEncodeFunc,
		"textdecodebase64": textDecodeBase64Func,
		"textencodebase64": textEncodeBase64Func,
		"urlencode":        urlEncodeFunc,
		"yamldecode":       yamlDecodeFunc,
		"yamlencode":       yamlEncodeFunc,

		// Filesystem functions
		"abspath":      abspathFunc(baseDir),
		"dirname":      dirnameFunc,
		"pathexpand":   pathExpandFunc,
		"basename":     basenameFunc,
		"file":         fileFunc(baseDir),
		"fileexists":   fileExistsFunc(baseDir),
		"fileset":      filesetFunc(baseDir),
		"filebase64":   fileBase64Func(baseDir),
		"templatefile": templateFileFunc(baseDir),

		// Date and time functions
		"formatdate": stdlib.FormatDateFunc,
		"timeadd":    timeAddFunc,
		"timecmp":    timeCmpFunc,
		"timestamp":  timestampFunc,

		// Hash and crypto functions
		"base64sha256":     base64Sha256Func,
		"base64sha512":     base64Sha512Func,
		"bcrypt":           bcryptFunc,
		"filebase64sha256": fileBase64Sha256Func(baseDir),
		"filebase64sha512": fileBase64Sha512Func(baseDir),
		"filemd5":          fileMd5Func(baseDir),
		"filesha1":         fileSha1Func(baseDir),
		"filesha256":       fileSha256Func(baseDir),
		"filesha512":       fileSha512Func(baseDir),
		"md5":              md5Func,
		"rsadecrypt":       rsaDecryptFunc,
		"sha1":             sha1Func,
		"sha256":           sha256Func,
		"sha512":           sha512Func,
		"uuid":             uuidFunc,
		"uuidv5":           uuidv5Func,

		// IP network functions
		"cidrhost":    cidrHostFunc,
		"cidrnetmask": cidrNetmaskFunc,
		"cidrsubnet":  cidrSubnetFunc,
		"cidrsubnets": cidrSubnetsFunc,

		// Type conversion functions
		"can":          canFunc,
		"issensitive":  issensitiveFunc,
		"nonsensitive": nonsensitiveFunc,
		"sensitive":    sensitiveFunc,
		"tobool":       toBoolFunc,
		"tolist":       toListFunc,
		"tomap":        toMapFunc,
		"tonumber":     toNumberFunc,
		"toset":        toSetFunc,
		"tostring":     toStringFunc,
		"try":          tryfunc.TryFunc,
		"type":         typeFunc,

		// Pulumi-specific functions
		"pulumiResourceName": pulumiResourceNameFunc,
		"pulumiResourceType": pulumiResourceTypeFunc,

		// Asset and archive functions
		"fileAsset":     fileAssetFunc(baseDir),
		"fileArchive":   fileArchiveFunc(baseDir),
		"stringAsset":   stringAssetFunc(),
		"assetArchive":  assetArchiveFunc(),
		"remoteAsset":   remoteAssetFunc(),
		"remoteArchive": remoteArchiveFunc(),
	}

	// templatestring renders an arbitrary string as a template. The functions
	// available inside that template are every function except the template
	// functions themselves, which keeps a template from invoking itself
	// recursively without bound.
	nestedTemplateFuncs := make(map[string]function.Function, len(funcs))
	for name, fn := range funcs {
		if name == "templatefile" || name == "templatestring" {
			continue
		}
		nestedTemplateFuncs[name] = fn
	}
	funcs["templatestring"] = templateStringFunc(nestedTemplateFuncs)

	return funcs
}

var canFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "expression",
			Type: customdecode.ExpressionClosureType,
		},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		closure := customdecode.ExpressionClosureFromVal(args[0])
		v, diags := closure.Value()
		var marks cty.ValueMarks
		if v.HasMark(SensitiveMark) {
			marks = cty.NewValueMarks(SensitiveMark)
		}

		if diags.HasErrors() {
			return cty.False.WithMarks(marks), nil
		}

		if !v.IsWhollyKnown() {
			// If the value is not wholly known, we still cannot be certain that
			// the expression was valid. There may be yet index expressions which
			// will fail once values are completely known.
			return cty.UnknownVal(cty.Bool).WithMarks(marks), nil
		}

		return cty.True.WithMarks(marks), nil
	},
})

// String functions

var startsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
		{Name: "prefix", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		str := args[0].AsString()
		prefix := args[1].AsString()
		return cty.BoolVal(strings.HasPrefix(str, prefix)), nil
	},
})

var endsWithFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
		{Name: "suffix", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		str := args[0].AsString()
		suffix := args[1].AsString()
		return cty.BoolVal(strings.HasSuffix(str, suffix)), nil
	},
})

var strContainsFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
		{Name: "substr", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		str := args[0].AsString()
		substr := args[1].AsString()
		return cty.BoolVal(strings.Contains(str, substr)), nil
	},
})

// Collection functions

var allTrueFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.List(cty.Bool)},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		for it := args[0].ElementIterator(); it.Next(); {
			_, v := it.Element()
			if !v.IsKnown() {
				return cty.UnknownVal(cty.Bool), nil
			}
			if v.False() {
				return cty.False, nil
			}
		}
		return cty.True, nil
	},
})

var anyTrueFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.List(cty.Bool)},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		for it := args[0].ElementIterator(); it.Next(); {
			_, v := it.Element()
			if !v.IsKnown() {
				return cty.UnknownVal(cty.Bool), nil
			}
			if v.True() {
				return cty.True, nil
			}
		}
		return cty.False, nil
	},
})

var lengthFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name:             "value",
			Type:             cty.DynamicPseudoType,
			AllowDynamicType: true,
			AllowUnknown:     true,
			AllowMarked:      true,
		},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		ty := args[0].Type()
		switch {
		case ty == cty.String, ty == cty.DynamicPseudoType,
			ty.IsTupleType(), ty.IsObjectType(),
			ty.IsListType(), ty.IsMapType(), ty.IsSetType():
			return cty.Number, nil
		default:
			return cty.Number, fmt.Errorf("argument must be a string, a collection type, or a structural type")
		}
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		v := args[0]
		ty := v.Type()
		switch {
		case ty == cty.DynamicPseudoType:
			return cty.UnknownVal(cty.Number), nil
		case ty == cty.String:
			return stdlib.Strlen(v)
		case ty.IsTupleType():
			return cty.NumberIntVal(int64(len(ty.TupleElementTypes()))), nil
		case ty.IsObjectType():
			return cty.NumberIntVal(int64(len(ty.AttributeTypes()))), nil
		case ty.IsListType(), ty.IsMapType(), ty.IsSetType():
			return v.Length(), nil
		default:
			return cty.UnknownVal(cty.Number), fmt.Errorf("impossible value type for length(...)")
		}
	},
})

var indexFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.DynamicPseudoType},
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		list := args[0]
		value := args[1]
		i := 0
		for it := list.ElementIterator(); it.Next(); {
			_, v := it.Element()
			if !v.IsWhollyKnown() {
				return cty.UnknownVal(cty.Number), nil
			}
			if v.Equals(value).True() {
				return cty.NumberIntVal(int64(i)), nil
			}
			i++
		}
		return cty.NilVal, fmt.Errorf("value not found in list")
	},
})

var listFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name: "elements",
		Type: cty.DynamicPseudoType,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		if len(args) == 0 {
			return cty.List(cty.DynamicPseudoType), nil
		}
		return cty.List(args[0].Type()), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		if len(args) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType), nil
		}
		return cty.ListVal(args), nil
	},
})

var lookupFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "map", Type: cty.DynamicPseudoType},
		{Name: "key", Type: cty.String},
	},
	VarParam: &function.Parameter{
		Name:             "default",
		Type:             cty.DynamicPseudoType,
		AllowNull:        true,
		AllowUnknown:     true,
		AllowDynamicType: true,
	},
	Type: function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		m := args[0]
		key := args[1].AsString()

		switch {
		case m.Type().IsMapType():
			idx := cty.StringVal(key)
			if m.HasIndex(idx).True() {
				return m.Index(idx), nil
			}
		case m.Type().IsObjectType():
			if m.Type().HasAttribute(key) {
				return m.GetAttr(key), nil
			}
		}

		// Return the default value, if any
		if len(args) > 2 {
			return args[2], nil
		}

		return cty.NilVal, fmt.Errorf("key %q not found", key)
	},
})

var mapFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name: "pairs",
		Type: cty.DynamicPseudoType,
	},
	Type: function.StaticReturnType(cty.Map(cty.DynamicPseudoType)),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		if len(args)%2 != 0 {
			return cty.NilVal, fmt.Errorf("map requires an even number of arguments")
		}
		m := make(map[string]cty.Value)
		for i := 0; i < len(args); i += 2 {
			key := args[i].AsString()
			m[key] = args[i+1]
		}
		if len(m) == 0 {
			return cty.MapValEmpty(cty.DynamicPseudoType), nil
		}
		return cty.MapVal(m), nil
	},
})

var matchkeysFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "values", Type: cty.List(cty.DynamicPseudoType)},
		{Name: "keys", Type: cty.List(cty.DynamicPseudoType)},
		{Name: "searchset", Type: cty.List(cty.DynamicPseudoType)},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		values := args[0]
		keys := args[1]
		searchset := args[2]

		searchMap := make(map[string]bool)
		for it := searchset.ElementIterator(); it.Next(); {
			_, v := it.Element()
			searchMap[v.GoString()] = true
		}

		var result []cty.Value
		valIt := values.ElementIterator()
		keyIt := keys.ElementIterator()
		for valIt.Next() && keyIt.Next() {
			_, v := valIt.Element()
			_, k := keyIt.Element()
			if searchMap[k.GoString()] {
				result = append(result, v)
			}
		}

		if len(result) == 0 {
			return cty.ListValEmpty(values.Type().ElementType()), nil
		}
		return cty.ListVal(result), nil
	},
})

var oneFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		ty := args[0].Type()
		if ty.IsListType() || ty.IsSetType() {
			return ty.ElementType(), nil
		}
		if ty.IsTupleType() {
			etys := ty.TupleElementTypes()
			if len(etys) == 0 {
				return cty.DynamicPseudoType, nil
			}
			return etys[0], nil
		}
		return cty.DynamicPseudoType, nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		list := args[0]
		if list.LengthInt() == 0 {
			return cty.NullVal(retType), nil
		}
		if list.LengthInt() > 1 {
			return cty.NilVal, fmt.Errorf("list has more than one element")
		}
		for it := list.ElementIterator(); it.Next(); {
			_, v := it.Element()
			return v, nil
		}
		return cty.NullVal(retType), nil
	},
})

// coalesceFunc returns the first argument that is neither null nor an empty
// string. cty's stdlib.CoalesceFunc only skips null, so it would return an empty
// string where Terraform skips it. Other "zero" values (the number 0, empty
// collections) are values in their own right and are returned as-is.
var coalesceFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name:             "vals",
		Type:             cty.DynamicPseudoType,
		AllowUnknown:     true,
		AllowDynamicType: true,
		AllowNull:        true,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		argTypes := make([]cty.Type, len(args))
		for i, val := range args {
			argTypes[i] = val.Type()
		}
		retType, _ := convert.UnifyUnsafe(argTypes)
		if retType == cty.NilType {
			return cty.NilType, fmt.Errorf("all arguments must have the same type")
		}
		return retType, nil
	},
	RefineResult: func(b *cty.RefinementBuilder) *cty.RefinementBuilder { return b.NotNull() },
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		for _, argVal := range args {
			argVal, _ = convert.Convert(argVal, retType)
			if !argVal.IsKnown() {
				return cty.UnknownVal(retType), nil
			}
			if argVal.IsNull() {
				continue
			}
			if retType == cty.String && argVal.RawEquals(cty.StringVal("")) {
				continue
			}
			return argVal, nil
		}
		return cty.NilVal, fmt.Errorf("no non-null, non-empty-string arguments")
	},
})

var sumFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.List(cty.Number)},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		if args[0].LengthInt() == 0 {
			// Matching OpenTofu's behavior, we error on an empty list
			return cty.NilVal, fmt.Errorf("cannot sum an empty list")
		}
		elements := args[0].AsValueSlice()
		sum := elements[0]
		for _, v := range elements[1:] {
			sum = sum.Add(v)
		}
		return sum, nil
	},
})

var entriesFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "collection", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		ret := func(k, v cty.Type) (cty.Type, error) {
			return cty.List(cty.Object(map[string]cty.Type{
				"key":   k,
				"value": v,
			})), nil
		}

		t := args[0].Type()

		switch {
		case t.IsObjectType():
			// We can't return a list, since that would require that each "value" type is the same. We can't
			// return a tuple since we don't know the length given option fields.
			return cty.DynamicPseudoType, nil
		case t.IsMapType():
			return ret(cty.String, args[0].Type().ElementType())
		case t.IsListType(), t.IsTupleType():
			return ret(cty.Number, args[0].Type().ElementType())
		default:
			return cty.NilType, fmt.Errorf("entries: expected a Map, Object or List")
		}

	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		v := args[0]
		t := v.Type()

		if !v.IsKnown() {
			return cty.UnknownVal(t), nil
		}

		if !t.IsCollectionType() && !t.IsObjectType() {
			return cty.Value{}, fmt.Errorf("entries: invalid input: %v", t)
		}

		elems := make([]cty.Value, 0, v.LengthInt())
		for it := v.ElementIterator(); it.Next(); {
			k, v := it.Element()
			elems = append(elems, cty.ObjectVal(map[string]cty.Value{
				"key":   k,
				"value": v,
			}))
		}

		if t.IsObjectType() || t.IsTupleType() {
			return cty.TupleVal(elems), nil
		}
		return cty.ListVal(elems), nil
	},
})

var transposeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "map", Type: cty.Map(cty.List(cty.String))},
	},
	Type: function.StaticReturnType(cty.Map(cty.List(cty.String))),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		result := make(map[string][]string)
		for it := args[0].ElementIterator(); it.Next(); {
			k, v := it.Element()
			key := k.AsString()
			for vit := v.ElementIterator(); vit.Next(); {
				_, val := vit.Element()
				valStr := val.AsString()
				result[valStr] = append(result[valStr], key)
			}
		}

		ctyResult := make(map[string]cty.Value)
		for k, v := range result {
			sort.Strings(v)
			vals := make([]cty.Value, len(v))
			for i, s := range v {
				vals[i] = cty.StringVal(s)
			}
			ctyResult[k] = cty.ListVal(vals)
		}

		if len(ctyResult) == 0 {
			return cty.MapValEmpty(cty.List(cty.String)), nil
		}
		return cty.MapVal(ctyResult), nil
	},
})

// Encoding functions

var base64DecodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		decoded, err := base64.StdEncoding.DecodeString(args[0].AsString())
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(decoded)), nil
	},
})

var base64EncodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		encoded := base64.StdEncoding.EncodeToString([]byte(args[0].AsString()))
		return cty.StringVal(encoded), nil
	},
})

var base64GzipFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		var buf bytes.Buffer
		w := gzip.NewWriter(&buf)
		if _, err := w.Write([]byte(args[0].AsString())); err != nil {
			return cty.NilVal, err
		}
		if err := w.Close(); err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(base64.StdEncoding.EncodeToString(buf.Bytes())), nil
	},
})

var textDecodeBase64Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
		{Name: "encoding", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		decoded, err := base64.StdEncoding.DecodeString(args[0].AsString())
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(decoded)), nil
	},
})

var textEncodeBase64Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
		{Name: "encoding", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		encoded := base64.StdEncoding.EncodeToString([]byte(args[0].AsString()))
		return cty.StringVal(encoded), nil
	},
})

var urlEncodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(url.QueryEscape(args[0].AsString())), nil
	},
})

var yamlDecodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		var data any
		if err := yaml.Unmarshal([]byte(args[0].AsString()), &data); err != nil {
			return cty.NilVal, err
		}
		return goToCty(data), nil
	},
})

var yamlEncodeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		if !args[0].IsWhollyKnown() {
			return cty.UnknownVal(retType), nil
		}
		data := ctyToGo(args[0])
		out, err := yaml.Marshal(data)
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(out)), nil
	},
})

// Filesystem functions

func abspathFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			absPath, err := filepath.Abs(path)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(absPath), nil
		},
	})
}

var dirnameFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "path", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(filepath.Dir(args[0].AsString())), nil
	},
})

var pathExpandFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "path", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		path := args[0].AsString()
		if strings.HasPrefix(path, "~") {
			home, err := os.UserHomeDir()
			if err != nil {
				return cty.NilVal, err
			}
			path = filepath.Join(home, path[1:])
		}
		return cty.StringVal(path), nil
	},
})

var basenameFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "path", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(filepath.Base(args[0].AsString())), nil
	},
})

func fileFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(string(content)), nil
		},
	})
}

func fileExistsFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.Bool),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			_, err := os.Stat(path)
			return cty.BoolVal(err == nil), nil
		},
	})
}

func filesetFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
			{Name: "pattern", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.Set(cty.String)),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			pattern := args[1].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			matches, err := filepath.Glob(filepath.Join(path, pattern))
			if err != nil {
				return cty.NilVal, err
			}
			vals := make([]cty.Value, len(matches))
			for i, m := range matches {
				rel, _ := filepath.Rel(path, m)
				vals[i] = cty.StringVal(rel)
			}
			if len(vals) == 0 {
				return cty.SetValEmpty(cty.String), nil
			}
			return cty.SetVal(vals), nil
		},
	})
}

func fileBase64Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(base64.StdEncoding.EncodeToString(content)), nil
		},
	})
}

func templateFileFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
			{Name: "vars", Type: cty.DynamicPseudoType},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}

			// Simple template substitution for ${var} patterns
			vars := args[1]
			result := string(content)
			if vars.Type().IsObjectType() || vars.Type().IsMapType() {
				for it := vars.ElementIterator(); it.Next(); {
					k, v := it.Element()
					key := k.AsString()
					var val string
					if v.Type() == cty.String {
						val = v.AsString()
					} else {
						val = v.GoString()
					}
					result = strings.ReplaceAll(result, "${"+key+"}", val)
				}
			}
			return cty.StringVal(result), nil
		},
	})
}

// templateStringFunc renders its first argument as an HCL template using the
// variables in its second argument. nestedFuncs is the function table made
// available inside the template.
func templateStringFunc(nestedFuncs map[string]function.Function) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "template", Type: cty.String},
			{Name: "vars", Type: cty.DynamicPseudoType},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			return renderTemplate(args[0], args[1], nestedFuncs)
		},
	})
}

// renderTemplate parses templateVal as an HCL template and evaluates it with
// varsVal's entries in scope plus the supplied functions, returning the
// rendered string. Marks on the inputs propagate to the result.
func renderTemplate(
	templateVal, varsVal cty.Value, funcs map[string]function.Function,
) (cty.Value, error) {
	templateVal, tmplMarks := templateVal.Unmark()

	expr, diags := hclsyntax.ParseTemplate(
		[]byte(templateVal.AsString()), "<templatestring>", hcl.InitialPos)
	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	vars := map[string]cty.Value{}
	if !varsVal.IsNull() {
		if ty := varsVal.Type(); !ty.IsObjectType() && !ty.IsMapType() {
			return cty.NilVal, fmt.Errorf(
				"vars argument must be an object, got %s", ty.FriendlyName())
		}
		for it := varsVal.ElementIterator(); it.Next(); {
			k, v := it.Element()
			vars[k.AsString()] = v
		}
	}

	val, diags := expr.Value(&hcl.EvalContext{Variables: vars, Functions: funcs})
	if diags.HasErrors() {
		return cty.NilVal, diags
	}

	val, err := convert.Convert(val, cty.String)
	if err != nil {
		return cty.NilVal, err
	}
	return val.WithMarks(tmplMarks), nil
}

// Date and time functions

var timeAddFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "timestamp", Type: cty.String},
		{Name: "duration", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		ts := args[0].AsString()
		durStr := args[1].AsString()

		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid timestamp: %s", err)
		}

		dur, err := time.ParseDuration(durStr)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid duration: %s", err)
		}

		return cty.StringVal(t.Add(dur).Format(time.RFC3339)), nil
	},
})

var timeCmpFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "timestamp_a", Type: cty.String},
		{Name: "timestamp_b", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		tsA := args[0].AsString()
		tsB := args[1].AsString()

		tA, err := time.Parse(time.RFC3339, tsA)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid timestamp_a: %s", err)
		}

		tB, err := time.Parse(time.RFC3339, tsB)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid timestamp_b: %s", err)
		}

		switch {
		case tA.Before(tB):
			return cty.NumberIntVal(-1), nil
		case tA.After(tB):
			return cty.NumberIntVal(1), nil
		default:
			return cty.NumberIntVal(0), nil
		}
	},
})

var timestampFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(time.Now().UTC().Format(time.RFC3339)), nil
	},
})

// Hash and crypto functions

var md5Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := md5.Sum([]byte(args[0].AsString()))
		return cty.StringVal(hex.EncodeToString(hash[:])), nil
	},
})

var sha1Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := sha1.Sum([]byte(args[0].AsString()))
		return cty.StringVal(hex.EncodeToString(hash[:])), nil
	},
})

var sha256Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := sha256.Sum256([]byte(args[0].AsString()))
		return cty.StringVal(hex.EncodeToString(hash[:])), nil
	},
})

var sha512Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := sha512.Sum512([]byte(args[0].AsString()))
		return cty.StringVal(hex.EncodeToString(hash[:])), nil
	},
})

var base64Sha256Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := sha256.Sum256([]byte(args[0].AsString()))
		return cty.StringVal(base64.StdEncoding.EncodeToString(hash[:])), nil
	},
})

var base64Sha512Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		hash := sha512.Sum512([]byte(args[0].AsString()))
		return cty.StringVal(base64.StdEncoding.EncodeToString(hash[:])), nil
	},
})

var bcryptFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "string", Type: cty.String},
	},
	VarParam: &function.Parameter{
		Name: "cost",
		Type: cty.Number,
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		cost := bcrypt.DefaultCost
		if len(args) > 1 {
			c, _ := args[1].AsBigFloat().Int64()
			cost = int(c)
		}
		hash, err := bcrypt.GenerateFromPassword([]byte(args[0].AsString()), cost)
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(hash)), nil
	},
})

func fileBase64Sha256Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := sha256.Sum256(content)
			return cty.StringVal(base64.StdEncoding.EncodeToString(hash[:])), nil
		},
	})
}

func fileBase64Sha512Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := sha512.Sum512(content)
			return cty.StringVal(base64.StdEncoding.EncodeToString(hash[:])), nil
		},
	})
}

func fileMd5Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := md5.Sum(content)
			return cty.StringVal(hex.EncodeToString(hash[:])), nil
		},
	})
}

func fileSha1Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := sha1.Sum(content)
			return cty.StringVal(hex.EncodeToString(hash[:])), nil
		},
	})
}

func fileSha256Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := sha256.Sum256(content)
			return cty.StringVal(hex.EncodeToString(hash[:])), nil
		},
	})
}

func fileSha512Func(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{
			{Name: "path", Type: cty.String},
		},
		Type: function.StaticReturnType(cty.String),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			if !filepath.IsAbs(path) {
				path = filepath.Join(baseDir, path)
			}
			content, err := os.ReadFile(path)
			if err != nil {
				return cty.NilVal, err
			}
			hash := sha512.Sum512(content)
			return cty.StringVal(hex.EncodeToString(hash[:])), nil
		},
	})
}

var uuidFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	Type:   function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(uuid.New().String()), nil
	},
})

var uuidv5Func = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "namespace", Type: cty.String},
		{Name: "name", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		nsStr := args[0].AsString()
		name := args[1].AsString()

		var ns uuid.UUID
		switch nsStr {
		case "dns", "DNS":
			ns = uuid.NameSpaceDNS
		case "url", "URL":
			ns = uuid.NameSpaceURL
		case "oid", "OID":
			ns = uuid.NameSpaceOID
		case "x500", "X500":
			ns = uuid.NameSpaceX500
		default:
			var err error
			ns, err = uuid.Parse(nsStr)
			if err != nil {
				return cty.NilVal, fmt.Errorf("invalid namespace: %s", err)
			}
		}

		return cty.StringVal(uuid.NewSHA1(ns, []byte(name)).String()), nil
	},
})

var rsaDecryptFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "ciphertext", Type: cty.String},
		{Name: "privatekey", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// Ciphertext is base64-encoded
		ciphertextB64 := args[0].AsString()
		privateKeyPEM := args[1].AsString()

		// Decode base64 ciphertext
		ciphertext, err := base64.StdEncoding.DecodeString(ciphertextB64)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid base64 ciphertext: %w", err)
		}

		// Parse PEM-encoded private key
		block, _ := pem.Decode([]byte(privateKeyPEM))
		if block == nil {
			return cty.NilVal, fmt.Errorf("invalid PEM-encoded private key")
		}

		// Parse the private key (supports PKCS1 and PKCS8)
		var privKey *rsa.PrivateKey
		if key, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
			privKey = key
		} else if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
			var ok bool
			privKey, ok = key.(*rsa.PrivateKey)
			if !ok {
				return cty.NilVal, fmt.Errorf("private key is not an RSA key")
			}
		} else {
			return cty.NilVal, fmt.Errorf("failed to parse private key")
		}

		// Decrypt using PKCS1v15 (Terraform's default)
		//nolint:staticcheck // SA1019: Using deprecated function for Terraform compatibility
		plaintext, err := rsa.DecryptPKCS1v15(rand.Reader, privKey, ciphertext)
		if err != nil {
			return cty.NilVal, fmt.Errorf("decryption failed: %w", err)
		}

		return cty.StringVal(string(plaintext)), nil
	},
})

// IP network functions

var cidrHostFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "prefix", Type: cty.String},
		{Name: "hostnum", Type: cty.Number},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		prefix := args[0].AsString()
		hostnum, _ := args[1].AsBigFloat().Int64()

		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid CIDR: %s", err)
		}

		ip := network.IP
		for i := len(ip) - 1; i >= 0 && hostnum > 0; i-- {
			ip[i] += byte(hostnum & 0xff)
			hostnum >>= 8
		}

		return cty.StringVal(ip.String()), nil
	},
})

var cidrNetmaskFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "prefix", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		prefix := args[0].AsString()

		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid CIDR: %s", err)
		}

		mask := net.IP(network.Mask)
		return cty.StringVal(mask.String()), nil
	},
})

var cidrSubnetFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "prefix", Type: cty.String},
		{Name: "newbits", Type: cty.Number},
		{Name: "netnum", Type: cty.Number},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		newbits, _ := args[1].AsBigFloat().Int64()
		netnum, _ := args[2].AsBigFloat().Int(nil)

		_, network, err := net.ParseCIDR(args[0].AsString())
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid CIDR: %s", err)
		}

		newNetwork, err := cidr.SubnetBig(network, int(newbits), netnum)
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(newNetwork.String()), nil
	},
})

// cidrSubnetsFunc allocates a sequence of consecutive subnets of the given
// prefix lengths (in additional bits) under a common base prefix. The
// allocator advances by each subnet's actual size, so mixed prefix lengths
// pack without overlap — matching Terraform's behaviour.
var cidrSubnetsFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "prefix", Type: cty.String},
	},
	VarParam: &function.Parameter{
		Name: "newbits",
		Type: cty.Number,
	},
	Type: function.StaticReturnType(cty.List(cty.String)),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		_, network, err := net.ParseCIDR(args[0].AsString())
		if err != nil {
			return cty.NilVal, function.NewArgErrorf(0, "invalid CIDR expression: %s", err)
		}
		startPrefixLen, _ := network.Mask.Size()

		newbitsArgs := args[1:]
		if len(newbitsArgs) == 0 {
			return cty.ListValEmpty(cty.String), nil
		}

		firstNewbits, _ := newbitsArgs[0].AsBigFloat().Int64()
		current, _ := cidr.PreviousSubnet(network, startPrefixLen+int(firstNewbits))

		out := make([]cty.Value, len(newbitsArgs))
		for i, arg := range newbitsArgs {
			newbits, _ := arg.AsBigFloat().Int64()
			if newbits < 1 {
				return cty.NilVal, function.NewArgErrorf(i+1, "must extend prefix by at least one bit")
			}
			length := startPrefixLen + int(newbits)
			if length > len(network.IP)*8 {
				protocol := "IP"
				switch len(network.IP) * 8 {
				case 32:
					protocol = "IPv4"
				case 128:
					protocol = "IPv6"
				}
				return cty.NilVal, function.NewArgErrorf(i+1,
					"would extend prefix to %d bits, which is too long for an %s address",
					length, protocol)
			}

			next, rollover := cidr.NextSubnet(current, length)
			if rollover || !network.Contains(next.IP) {
				return cty.NilVal, function.NewArgErrorf(i+1,
					"not enough remaining address space for a subnet with a prefix of %d bits after %s",
					length, current.String())
			}
			current = next
			out[i] = cty.StringVal(current.String())
		}
		return cty.ListVal(out), nil
	},
})

// Type conversion functions

var nonsensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType, AllowMarked: true},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		val, _ := args[0].Unmark()
		return val.Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val, marks := args[0].Unmark()
		delete(marks, "sensitive")
		if len(marks) == 0 {
			return val, nil
		}
		return val.WithMarks(marks), nil
	},
})

var sensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// Mark as sensitive (cty supports this via marks)
		return args[0].Mark("sensitive"), nil
	},
})

var issensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType, AllowMarked: true},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.BoolVal(args[0].HasMark(SensitiveMark)), nil
	},
})

var toBoolFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		if val.Type() == cty.Bool {
			return val, nil
		}
		if val.Type() == cty.String {
			s := val.AsString()
			switch strings.ToLower(s) {
			case "true", "1", "yes", "on":
				return cty.True, nil
			case "false", "0", "no", "off":
				return cty.False, nil
			default:
				return cty.NilVal, fmt.Errorf("cannot convert %q to bool", s)
			}
		}
		return cty.NilVal, fmt.Errorf("cannot convert %s to bool", val.Type().FriendlyName())
	},
})

var toListFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		ty := args[0].Type()
		if ty.IsSetType() {
			return cty.List(ty.ElementType()), nil
		}
		if ty.IsTupleType() {
			// Find common type
			return cty.List(cty.DynamicPseudoType), nil
		}
		return cty.List(cty.DynamicPseudoType), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		if val.Type().IsListType() {
			return val, nil
		}
		var vals []cty.Value
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			vals = append(vals, v)
		}
		if len(vals) == 0 {
			elemTy := cty.DynamicPseudoType
			if retType.IsListType() {
				elemTy = retType.ElementType()
			}
			return cty.ListValEmpty(elemTy), nil
		}
		return cty.ListVal(vals), nil
	},
})

var toMapFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.Map(cty.DynamicPseudoType)),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		if val.Type().IsMapType() {
			return val, nil
		}
		m := make(map[string]cty.Value)
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			m[k.AsString()] = v
		}
		if len(m) == 0 {
			return cty.MapValEmpty(cty.DynamicPseudoType), nil
		}
		return cty.MapVal(m), nil
	},
})

var toNumberFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		if val.Type() == cty.Number {
			return val, nil
		}
		if val.Type() == cty.String {
			s := val.AsString()
			// Try parsing as integer first
			var i int64
			if _, err := fmt.Sscanf(s, "%d", &i); err == nil {
				return cty.NumberIntVal(i), nil
			}
			// Try parsing as float
			var f float64
			if _, err := fmt.Sscanf(s, "%f", &f); err == nil {
				return cty.NumberFloatVal(f), nil
			}
			return cty.NilVal, fmt.Errorf("cannot convert %q to number", s)
		}
		return cty.NilVal, fmt.Errorf("cannot convert %s to number", val.Type().FriendlyName())
	},
})

var toSetFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		ty := args[0].Type()
		if ty.IsListType() {
			return cty.Set(ty.ElementType()), nil
		}
		return cty.Set(cty.DynamicPseudoType), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		if val.Type().IsSetType() {
			return val, nil
		}
		var vals []cty.Value
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			vals = append(vals, v)
		}
		if len(vals) == 0 {
			elemTy := cty.DynamicPseudoType
			if retType.IsSetType() {
				elemTy = retType.ElementType()
			}
			return cty.SetValEmpty(elemTy), nil
		}
		return cty.SetVal(vals), nil
	},
})

var toStringFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		val := args[0]
		switch val.Type() {
		case cty.String:
			return val, nil
		case cty.Number:
			return convert.Convert(val, cty.String)
		case cty.Bool:
			return cty.StringVal(fmt.Sprintf("%t", val.True())), nil
		// For complex types, JSON encode
		default:
			if !val.IsWhollyKnown() {
				return cty.UnknownVal(retType), nil
			}
			jsonBytes, err := json.Marshal(ctyToGo(val))
			if err != nil {
				return cty.NilVal, err
			}
			return cty.StringVal(string(jsonBytes)), nil
		}
	},
})

var typeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		return cty.StringVal(args[0].Type().FriendlyName()), nil
	},
})

// Helper to convert cty.Value to Go any
func ctyToGo(val cty.Value) any {
	if val.IsNull() {
		return nil
	}

	ty := val.Type()
	switch {
	case ty == cty.String:
		return val.AsString()
	case ty == cty.Number:
		f, _ := val.AsBigFloat().Float64()
		return f
	case ty == cty.Bool:
		return val.True()
	case ty.IsListType() || ty.IsTupleType() || ty.IsSetType():
		var result []any
		for it := val.ElementIterator(); it.Next(); {
			_, v := it.Element()
			result = append(result, ctyToGo(v))
		}
		return result
	case ty.IsMapType() || ty.IsObjectType():
		result := make(map[string]any)
		for it := val.ElementIterator(); it.Next(); {
			k, v := it.Element()
			result[k.AsString()] = ctyToGo(v)
		}
		return result
	default:
		return nil
	}
}

// Helper to convert Go any to cty.Value
func goToCty(val any) cty.Value {
	if val == nil {
		return cty.NullVal(cty.DynamicPseudoType)
	}

	switch v := val.(type) {
	case string:
		return cty.StringVal(v)
	case int:
		return cty.NumberIntVal(int64(v))
	case int64:
		return cty.NumberIntVal(v)
	case float64:
		return cty.NumberFloatVal(v)
	case bool:
		return cty.BoolVal(v)
	case []any:
		if len(v) == 0 {
			return cty.ListValEmpty(cty.DynamicPseudoType)
		}
		vals := make([]cty.Value, len(v))
		for i, item := range v {
			vals[i] = goToCty(item)
		}
		return cty.TupleVal(vals)
	case map[string]any:
		if len(v) == 0 {
			return cty.EmptyObjectVal
		}
		vals := make(map[string]cty.Value, len(v))
		for k, item := range v {
			vals[k] = goToCty(item)
		}
		return cty.ObjectVal(vals)
	default:
		return cty.NullVal(cty.DynamicPseudoType)
	}
}

// Ensure these are used (to avoid import errors)
var (
	_ = regexp.MustCompile
	_ = json.Marshal
	_ = csv.NewReader
)

// Pulumi-specific functions

// pulumiResourceNameFunc returns the logical name of a Pulumi resource by extracting it from the
// resource's URN. The argument must be a resource reference (an object with a "urn" attribute).
var pulumiResourceNameFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "resource",
			Type: cty.DynamicPseudoType,
		},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		res := args[0]
		if !res.IsKnown() {
			return cty.UnknownVal(cty.String), nil
		}
		if !res.Type().IsObjectType() || !res.Type().HasAttribute("urn") {
			return cty.NilVal, fmt.Errorf("pulumiResourceName: argument must be a resource reference")
		}
		urnVal := res.GetAttr("urn")
		if !urnVal.IsKnown() {
			return cty.UnknownVal(cty.String), nil
		}
		name, _, err := splitURN(urnVal.AsString())
		if err != nil {
			return cty.NilVal, fmt.Errorf("pulumiResourceName: %w", err)
		}
		return cty.StringVal(name), nil
	},
})

// pulumiResourceTypeFunc returns the type token of a Pulumi resource by extracting it from the
// resource's URN. The argument must be a resource reference (an object with a "urn" attribute).
var pulumiResourceTypeFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{
			Name: "resource",
			Type: cty.DynamicPseudoType,
		},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		res := args[0]
		if !res.IsKnown() {
			return cty.UnknownVal(cty.String), nil
		}
		if !res.Type().IsObjectType() || !res.Type().HasAttribute("urn") {
			return cty.NilVal, fmt.Errorf("pulumiResourceType: argument must be a resource reference")
		}
		urnVal := res.GetAttr("urn")
		if !urnVal.IsKnown() {
			return cty.UnknownVal(cty.String), nil
		}
		_, typeToken, err := splitURN(urnVal.AsString())
		if err != nil {
			return cty.NilVal, fmt.Errorf("pulumiResourceType: %w", err)
		}
		return cty.StringVal(typeToken), nil
	},
})

func splitURN(u string) (name, typeToken string, err error) {
	urn := urn.URN(u)
	if !urn.IsValid() {
		return "", "", fmt.Errorf("invalid Pulumi URN: %q", urn)
	}
	return urn.Name(), string(urn.Type()), nil
}

// Asset and archive functions

func fileAssetFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(AssetCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			a, err := asset.FromPathWithWD(path, baseDir)
			if err != nil {
				return cty.NilVal, fmt.Errorf("fileAsset: %w", err)
			}
			return cty.CapsuleVal(AssetCapsuleType, a), nil
		},
	})
}

func fileArchiveFunc(baseDir string) function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "path", Type: cty.String}},
		Type:   function.StaticReturnType(ArchiveCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			path := args[0].AsString()
			a, err := archive.FromPathWithWD(path, baseDir)
			if err != nil {
				return cty.NilVal, fmt.Errorf("fileArchive: %w", err)
			}
			return cty.CapsuleVal(ArchiveCapsuleType, a), nil
		},
	})
}

func stringAssetFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "text", Type: cty.String}},
		Type:   function.StaticReturnType(AssetCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			text := args[0].AsString()
			a := &asset.Asset{Sig: asset.AssetSig, Text: text}
			return cty.CapsuleVal(AssetCapsuleType, a), nil
		},
	})
}

func assetArchiveFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "assets", Type: cty.DynamicPseudoType}},
		Type:   function.StaticReturnType(ArchiveCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			val := args[0]
			if !val.Type().IsObjectType() && !val.Type().IsMapType() {
				return cty.NilVal, fmt.Errorf("assetArchive: argument must be an object or map")
			}
			assets := make(map[string]any)
			for it := val.ElementIterator(); it.Next(); {
				k, v := it.Element()
				key := k.AsString()
				switch {
				case v.Type().Equals(AssetCapsuleType):
					assets[key] = v.EncapsulatedValue().(*asset.Asset)
				case v.Type().Equals(ArchiveCapsuleType):
					assets[key] = v.EncapsulatedValue().(*archive.Archive)
				default:
					return cty.NilVal, fmt.Errorf("assetArchive: value for key %q must be an asset or archive, got %s",
						key, v.Type().FriendlyName())
				}
			}
			a := &archive.Archive{Sig: archive.ArchiveSig, Assets: assets}
			return cty.CapsuleVal(ArchiveCapsuleType, a), nil
		},
	})
}

func remoteAssetFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "uri", Type: cty.String}},
		Type:   function.StaticReturnType(AssetCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			uri := args[0].AsString()
			a, err := asset.FromURI(uri)
			if err != nil {
				return cty.NilVal, fmt.Errorf("remoteAsset: %w", err)
			}
			return cty.CapsuleVal(AssetCapsuleType, a), nil
		},
	})
}

func remoteArchiveFunc() function.Function {
	return function.New(&function.Spec{
		Params: []function.Parameter{{Name: "uri", Type: cty.String}},
		Type:   function.StaticReturnType(ArchiveCapsuleType),
		Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
			uri := args[0].AsString()
			a, err := archive.FromURI(uri)
			if err != nil {
				return cty.NilVal, fmt.Errorf("remoteArchive: %w", err)
			}
			return cty.CapsuleVal(ArchiveCapsuleType, a), nil
		},
	})
}
