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
	"math"
	"net"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/zclconf/go-cty/cty"
	"github.com/zclconf/go-cty/cty/function"
	"github.com/zclconf/go-cty/cty/function/stdlib"
	"golang.org/x/crypto/bcrypt"
	"gopkg.in/yaml.v3"
)

// Functions returns a map of all Terraform-compatible functions.
func Functions(baseDir string) map[string]function.Function {
	return map[string]function.Function{
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
		"indent":      indentFunc,
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
		"coalesce":        stdlib.CoalesceFunc,
		"coalescelist":    coalesceListFunc,
		"compact":         stdlib.CompactFunc,
		"concat":          stdlib.ConcatFunc,
		"contains":        stdlib.ContainsFunc,
		"distinct":        stdlib.DistinctFunc,
		"element":         stdlib.ElementFunc,
		"flatten":         stdlib.FlattenFunc,
		"index":           indexFunc,
		"keys":            stdlib.KeysFunc,
		"length":          stdlib.LengthFunc,
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
		"formatdate": formatDateFunc,
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
		"nonsensitive": nonsensitiveFunc,
		"sensitive":    sensitiveFunc,
		"tobool":       toBoolFunc,
		"tolist":       toListFunc,
		"tomap":        toMapFunc,
		"tonumber":     toNumberFunc,
		"toset":        toSetFunc,
		"tostring":     toStringFunc,
		"try":          tryFunc,
		"type":         typeFunc,
	}
}

// String functions

var indentFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "spaces", Type: cty.Number},
		{Name: "string", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		spaces := args[0].AsBigFloat()
		spacesInt, _ := spaces.Int64()
		str := args[1].AsString()
		indent := strings.Repeat(" ", int(spacesInt))
		lines := strings.Split(str, "\n")
		for i := range lines {
			if lines[i] != "" {
				lines[i] = indent + lines[i]
			}
		}
		return cty.StringVal(strings.Join(lines, "\n")), nil
	},
})

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
			if v.True() {
				return cty.True, nil
			}
		}
		return cty.False, nil
	},
})

var coalesceListFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name: "lists",
		Type: cty.DynamicPseudoType,
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		for _, arg := range args {
			if !arg.Type().IsListType() && !arg.Type().IsTupleType() {
				return cty.NilType, fmt.Errorf("arguments must be lists")
			}
		}
		return cty.DynamicPseudoType, nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		for _, arg := range args {
			if arg.LengthInt() > 0 {
				return arg, nil
			}
		}
		return cty.NilVal, fmt.Errorf("no non-empty list")
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
		Name: "default",
		Type: cty.DynamicPseudoType,
	},
	Type: function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		m := args[0]
		key := args[1].AsString()

		if m.Type().IsMapType() || m.Type().IsObjectType() {
			if m.Type().IsObjectType() {
				if m.Type().HasAttribute(key) {
					return m.GetAttr(key), nil
				}
			} else {
				idx := cty.StringVal(key)
				if m.HasIndex(idx).True() {
					return m.Index(idx), nil
				}
			}
		}

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

var sumFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "list", Type: cty.List(cty.Number)},
	},
	Type: function.StaticReturnType(cty.Number),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		sum := 0.0
		for it := args[0].ElementIterator(); it.Next(); {
			_, v := it.Element()
			f, _ := v.AsBigFloat().Float64()
			sum += f
		}
		return cty.NumberFloatVal(sum), nil
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
		// Simple URL encoding
		s := args[0].AsString()
		var result strings.Builder
		for _, r := range s {
			if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') ||
				r == '-' || r == '_' || r == '.' || r == '~' {
				result.WriteRune(r)
			} else {
				fmt.Fprintf(&result, "%%%02X", r)
			}
		}
		return cty.StringVal(result.String()), nil
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

// Date and time functions

var formatDateFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "spec", Type: cty.String},
		{Name: "timestamp", Type: cty.String},
	},
	Type: function.StaticReturnType(cty.String),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		spec := args[0].AsString()
		ts := args[1].AsString()

		t, err := time.Parse(time.RFC3339, ts)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid timestamp: %s", err)
		}

		// Convert Terraform's date format spec to Go format
		// Order matters - longer patterns must come before shorter ones
		replacements := []struct {
			from, to string
		}{
			{"YYYY", "2006"},
			{"YY", "06"},
			{"MMMM", "January"},
			{"MMM", "Jan"},
			{"MM", "01"},
			{"DD", "02"},
			{"EEEE", "Monday"},
			{"EEE", "Mon"},
			{"HH", "15"},
			{"hh", "03"},
			{"mm", "04"},
			{"ss", "05"},
			{"AA", "PM"},
			{"aa", "pm"},
			{"ZZZZ", "-07:00"},
			{"ZZZ", "MST"},
			{"Z", "-0700"},
			// Single character replacements must come last
			{"M", "1"},
			{"D", "2"},
			{"h", "3"},
			{"m", "4"},
			{"s", "5"},
		}
		result := spec
		for _, r := range replacements {
			result = strings.ReplaceAll(result, r.from, r.to)
		}

		return cty.StringVal(t.Format(result)), nil
	},
})

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
		prefix := args[0].AsString()
		newbits, _ := args[1].AsBigFloat().Int64()
		netnum, _ := args[2].AsBigFloat().Int64()

		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid CIDR: %s", err)
		}

		ones, bits := network.Mask.Size()
		newOnes := ones + int(newbits)
		if newOnes > bits {
			return cty.NilVal, fmt.Errorf("newbits %d would create prefix length %d, exceeding %d", newbits, newOnes, bits)
		}

		// Calculate the new network address
		// Convert IP to big-endian integer, add subnet offset, convert back
		ip := make(net.IP, len(network.IP))
		copy(ip, network.IP)

		// Calculate offset to add based on netnum and new prefix length
		// The offset is netnum * (size of new subnet in addresses)
		// For an IP address, we shift netnum left by (bits - newOnes) positions
		bitsToShift := uint(bits - newOnes)

		// Convert netnum to bytes and add to IP
		offset := netnum << bitsToShift
		for i := len(ip) - 1; i >= 0 && offset > 0; i-- {
			sum := int64(ip[i]) + (offset & 0xff)
			ip[i] = byte(sum & 0xff)
			offset >>= 8
		}

		newNet := &net.IPNet{
			IP:   ip,
			Mask: net.CIDRMask(newOnes, bits),
		}

		return cty.StringVal(newNet.String()), nil
	},
})

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
		prefix := args[0].AsString()

		_, network, err := net.ParseCIDR(prefix)
		if err != nil {
			return cty.NilVal, fmt.Errorf("invalid CIDR: %s", err)
		}

		ones, bits := network.Mask.Size()
		var subnets []cty.Value

		var netnum int64
		for i := 1; i < len(args); i++ {
			newbits, _ := args[i].AsBigFloat().Int64()
			newOnes := ones + int(newbits)
			if newOnes > bits {
				return cty.NilVal, fmt.Errorf("newbits %d would create prefix length %d, exceeding %d", newbits, newOnes, bits)
			}

			ip := make(net.IP, len(network.IP))
			copy(ip, network.IP)

			// Add netnum to the IP
			shift := uint(bits - newOnes)
			n := netnum
			for j := len(ip) - 1; j >= 0 && n > 0; j-- {
				add := byte((n << shift) & 0xff)
				ip[j] |= add
				n >>= (8 - shift)
				if shift >= 8 {
					shift -= 8
				} else {
					shift = 0
				}
			}

			newNet := &net.IPNet{
				IP:   ip,
				Mask: net.CIDRMask(newOnes, bits),
			}
			subnets = append(subnets, cty.StringVal(newNet.String()))
			netnum++
		}

		if len(subnets) == 0 {
			return cty.ListValEmpty(cty.String), nil
		}
		return cty.ListVal(subnets), nil
	},
})

// Type conversion functions

var canFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "expression", Type: cty.DynamicPseudoType},
	},
	Type: function.StaticReturnType(cty.Bool),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// The expression already evaluated successfully if we got here
		return cty.True, nil
	},
})

var nonsensitiveFunc = function.New(&function.Spec{
	Params: []function.Parameter{
		{Name: "value", Type: cty.DynamicPseudoType},
	},
	Type: func(args []cty.Value) (cty.Type, error) {
		return args[0].Type(), nil
	},
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// In our implementation, we don't track sensitivity at the cty level
		return args[0], nil
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
			return cty.ListValEmpty(cty.DynamicPseudoType), nil
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
			return cty.SetValEmpty(cty.DynamicPseudoType), nil
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
		if val.Type() == cty.String {
			return val, nil
		}
		if val.Type() == cty.Number {
			f, _ := val.AsBigFloat().Float64()
			if f == math.Trunc(f) {
				return cty.StringVal(fmt.Sprintf("%d", int64(f))), nil
			}
			return cty.StringVal(fmt.Sprintf("%g", f)), nil
		}
		if val.Type() == cty.Bool {
			return cty.StringVal(fmt.Sprintf("%t", val.True())), nil
		}
		// For complex types, JSON encode
		jsonBytes, err := json.Marshal(ctyToGo(val))
		if err != nil {
			return cty.NilVal, err
		}
		return cty.StringVal(string(jsonBytes)), nil
	},
})

var tryFunc = function.New(&function.Spec{
	Params: []function.Parameter{},
	VarParam: &function.Parameter{
		Name: "expressions",
		Type: cty.DynamicPseudoType,
	},
	Type: function.StaticReturnType(cty.DynamicPseudoType),
	Impl: func(args []cty.Value, retType cty.Type) (cty.Value, error) {
		// Return the first non-null value
		for _, arg := range args {
			if !arg.IsNull() {
				return arg, nil
			}
		}
		return cty.NilVal, fmt.Errorf("all expressions failed")
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
