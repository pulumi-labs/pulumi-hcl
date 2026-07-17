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
	"crypto/rand"
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/hashicorp/hcl/v2"
	"github.com/hashicorp/hcl/v2/hclsyntax"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/zclconf/go-cty/cty"
	"golang.org/x/crypto/ssh"
)

func evalExpr(t *testing.T, baseDir, src string) cty.Value {
	expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("Failed to parse expression %q: %s", src, diags.Error())
	}

	ctx := &hcl.EvalContext{
		Functions: Functions(baseDir),
	}

	val, diags := expr.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("Failed to evaluate expression %q: %s", src, diags.Error())
	}
	return val
}

func TestRecoverFunction(t *testing.T) {
	t.Parallel()

	// thing has a "present" attribute but not "missing", so `thing.missing` is an
	// evaluation error that recover catches — standing in for a reference to a
	// resource that failed to create.
	thing := cty.ObjectVal(map[string]cty.Value{"present": cty.StringVal("v")})
	eval := func(src string) cty.Value {
		expr, diags := hclsyntax.ParseExpression([]byte(src), "test.hcl", hcl.Pos{Line: 1, Column: 1})
		require.False(t, diags.HasErrors(), diags.Error())
		val, diags := expr.Value(&hcl.EvalContext{
			Functions: Functions("/tmp"),
			Variables: map[string]cty.Value{"thing": thing},
		})
		require.False(t, diags.HasErrors(), diags.Error())
		return val
	}

	// When the value evaluates successfully, recover returns it and never
	// evaluates the recovery expression.
	assert.Equal(t, cty.StringVal("v"), eval(`recover(thing.present, "fallback")`))

	// When the value fails, recover evaluates the recovery expression with
	// `error` bound to the value's actual evaluation error.
	assert.Equal(t, cty.StringVal(
		`recovered: test.hcl:1,14-22: Unsupported attribute; `+
			`This object does not have an attribute named "missing".`),
		eval(`recover(thing.missing, "recovered: ${error}")`))

	assert.Equal(t, cty.True, eval(`recover(thing.missing, error != "")`))
}

func TestStringFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		// join
		{"join basic", `join(", ", ["a", "b", "c"])`, cty.StringVal("a, b, c")},
		{"join empty", `join("-", [])`, cty.StringVal("")},

		// split
		{"split basic", `split(",", "a,b,c")`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// lower/upper
		{"lower", `lower("HELLO")`, cty.StringVal("hello")},
		{"upper", `upper("hello")`, cty.StringVal("HELLO")},

		// trim functions
		{"trim", `trim("  hello  ", " ")`, cty.StringVal("hello")},
		{"trimprefix", `trimprefix("helloworld", "hello")`, cty.StringVal("world")},
		{"trimsuffix", `trimsuffix("helloworld", "world")`, cty.StringVal("hello")},
		{"trimspace", `trimspace("  hello  ")`, cty.StringVal("hello")},

		// replace
		{"replace", `replace("hello world", "world", "there")`, cty.StringVal("hello there")},
		{"replace regex", `replace("hello world", "/o./", "X")`, cty.StringVal("hellXwXld")},
		{"replace regex anchored", `replace("foo bar foo", "/^foo/", "Z")`, cty.StringVal("Z bar foo")},
		{"replace regex class", `replace("a1b2c3", "/[0-9]/", "_")`, cty.StringVal("a_b_c_")},
		{"replace regex inner slashes", `replace("a/b/c", "/b/", "X")`, cty.StringVal("a/X/c")},
		{"replace literal dot", `replace("a.b.c", ".", "-")`, cty.StringVal("a-b-c")},

		// substr
		{"substr", `substr("hello", 0, 3)`, cty.StringVal("hel")},

		// format
		{"format basic", `format("Hello, %s!", "World")`, cty.StringVal("Hello, World!")},
		{"format number", `format("Count: %d", 42)`, cty.StringVal("Count: 42")},

		// chomp
		{"chomp", `chomp("hello\n")`, cty.StringVal("hello")},

		// indent pads after each newline, leaving the first line untouched.
		{"indent", `indent(2, "hello\nworld")`, cty.StringVal("hello\n  world")},
		{"indent pads blank line", `indent(2, "a\n\nb")`, cty.StringVal("a\n  \n  b")},

		// title
		{"title", `title("hello world")`, cty.StringVal("Hello World")},

		// regex
		{"regex", `regex("\\d+", "abc123def")`, cty.StringVal("123")},
		{"regexall", `length(regexall("\\d+", "123-456-789"))`, cty.NumberIntVal(3)},

		// startswith/endswith
		{"startswith true", `startswith("hello", "hel")`, cty.BoolVal(true)},
		{"startswith false", `startswith("hello", "world")`, cty.BoolVal(false)},
		{"endswith true", `endswith("hello", "lo")`, cty.BoolVal(true)},
		{"endswith false", `endswith("hello", "he")`, cty.BoolVal(false)},

		// strcontains
		{"strcontains true", `strcontains("hello world", "wor")`, cty.BoolVal(true)},
		{"strcontains false", `strcontains("hello world", "xyz")`, cty.BoolVal(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestTemplateStringFunction(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{
			"substitution",
			`templatestring("Hello, $${name}!", { name = "Ada" })`,
			cty.StringVal("Hello, Ada!"),
		},
		{
			"number coerced to string",
			`templatestring("count=$${n}", { n = 3 })`,
			cty.StringVal("count=3"),
		},
		{
			"function call inside template",
			`templatestring("$${upper(who)}", { who = "bob" })`,
			cty.StringVal("BOB"),
		},
		{
			"for directive",
			`templatestring("items:%%{ for n in names } $${n}%%{ endfor }", { names = ["a", "b", "c"] })`,
			cty.StringVal("items: a b c"),
		},
		{
			"if directive",
			`templatestring("$${name}%%{ if admin } (admin)%%{ endif }", { name = "Ada", admin = true })`,
			cty.StringVal("Ada (admin)"),
		},
		{
			"empty vars",
			`templatestring("static", {})`,
			cty.StringVal("static"),
		},
		{
			"single interpolation preserves tuple",
			`templatestring("$${a}", { a = ["foo", "bar"] })`,
			cty.TupleVal([]cty.Value{cty.StringVal("foo"), cty.StringVal("bar")}),
		},
		{
			"single interpolation preserves number",
			`templatestring("$${a}", { a = 42 })`,
			cty.NumberIntVal(42),
		},
		{
			"single interpolation preserves bool",
			`templatestring("$${a}", { a = true })`,
			cty.True,
		},
		{
			"single interpolation preserves object",
			`templatestring("$${a}", { a = { k = 1 } })`,
			cty.ObjectVal(map[string]cty.Value{"k": cty.NumberIntVal(1)}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestCollectionFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"length list", `length(["a", "b", "c"])`, cty.NumberIntVal(3)},
		{"length tuple", `length(tolist(["a", "b"]))`, cty.NumberIntVal(2)},
		{"length string", `length("hello")`, cty.NumberIntVal(5)},
		{"length empty string", `length("")`, cty.NumberIntVal(0)},
		{"length map", `length({a = 1, b = 2})`, cty.NumberIntVal(2)},
		{"length object", `length({a = 1, b = "two", c = true})`, cty.NumberIntVal(3)},

		// element
		{"element", `element(["a", "b", "c"], 1)`, cty.StringVal("b")},
		{"element wrap", `element(["a", "b", "c"], 4)`, cty.StringVal("b")}, // wraps around

		// index
		{"index", `index(["a", "b", "c"], "b")`, cty.NumberIntVal(1)},

		// lookup
		{"lookup found", `lookup({a = "x", b = "y"}, "a", "default")`, cty.StringVal("x")},
		{"lookup default", `lookup({a = "x"}, "b", "default")`, cty.StringVal("default")},
		// On the map path the default is converted to the element type, just
		// like OpenTofu.
		{"lookup map default num to string", `lookup(tomap({a = "1"}), "missing", 30)`, cty.StringVal("30")},
		{"lookup map default string to num", `lookup(tomap({a = 1}), "missing", "80")`, cty.NumberIntVal(80)},
		{"lookup map default bool to string", `lookup(tomap({a = "x"}), "missing", true)`, cty.StringVal("true")},
		// On the object path the default keeps its own type.
		{"lookup object default unconverted", `lookup({a = "x"}, "missing", 30)`, cty.NumberIntVal(30)},
		// A present key never consults the default, so a sensitive default
		// leaves the returned value unmarked.
		{"lookup sensitive default unused", `lookup({a = "x"}, "a", sensitive("d"))`, cty.StringVal("x")},
		// A returned default carries its own sensitivity.
		{"lookup sensitive default used", `lookup({a = "x"}, "b", sensitive("d"))`, cty.StringVal("d").Mark(SensitiveMark)},
		// Collection and key sensitivity always propagate to the result.
		{"lookup sensitive element", `lookup({a = sensitive("x")}, "a", "d")`, cty.StringVal("x").Mark(SensitiveMark)},
		{"lookup sensitive key", `lookup({a = "x"}, sensitive("a"), "d")`, cty.StringVal("x").Mark(SensitiveMark)},

		// contains
		{"contains true", `contains(["a", "b"], "a")`, cty.BoolVal(true)},
		{"contains false", `contains(["a", "b"], "c")`, cty.BoolVal(false)},

		// keys/values
		{"keys", `sort(keys({b = 1, a = 2}))`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"),
		})},

		// merge
		{"merge", `merge({a = 1}, {b = 2})`, cty.ObjectVal(map[string]cty.Value{
			"a": cty.NumberIntVal(1),
			"b": cty.NumberIntVal(2),
		})},

		// concat
		{"concat", `concat(["a"], ["b", "c"])`, cty.TupleVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// flatten
		{"flatten", `flatten([["a"], ["b", "c"]])`, cty.TupleVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// distinct
		{"distinct", `distinct(["a", "b", "a", "c"])`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// reverse
		{"reverse", `reverse(["a", "b", "c"])`, cty.TupleVal([]cty.Value{
			cty.StringVal("c"), cty.StringVal("b"), cty.StringVal("a"),
		})},

		// sort
		{"sort", `sort(["c", "a", "b"])`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// compact
		{"compact", `compact(["a", "", "b", ""])`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"),
		})},

		// coalesce returns the first non-null, non-empty-string argument.
		{"coalesce", `coalesce("a", "b")`, cty.StringVal("a")},
		{"coalesce skips empty string", `coalesce("", "b")`, cty.StringVal("b")},
		{"coalesce skips null then empty", `coalesce(null, "", "w")`, cty.StringVal("w")},
		{"coalesce keeps zero", `coalesce(0, 5)`, cty.NumberIntVal(0)},

		// coalescelist (returns tuple type not list)
		{"coalescelist", `length(coalescelist([], ["a"]))`, cty.NumberIntVal(1)},

		// range
		{"range simple", `range(3)`, cty.ListVal([]cty.Value{
			cty.NumberIntVal(0), cty.NumberIntVal(1), cty.NumberIntVal(2),
		})},
		{"range start end", `range(1, 4)`, cty.ListVal([]cty.Value{
			cty.NumberIntVal(1), cty.NumberIntVal(2), cty.NumberIntVal(3),
		})},

		// slice (returns tuple type)
		{"slice", `length(slice(["a", "b", "c", "d"], 1, 3))`, cty.NumberIntVal(2)},

		// chunklist
		{"chunklist", `length(chunklist(["a", "b", "c", "d", "e"], 2))`, cty.NumberIntVal(3)},

		// one
		{"one single", `one(["hello"])`, cty.StringVal("hello")},
		{"one empty", `one([])`, cty.NullVal(cty.DynamicPseudoType)},

		// sum accumulates with arbitrary precision, so the exact result survives
		// even when an input is not representable as a float64.
		{"sum", `sum([1, 2, 3, 4])`, cty.NumberIntVal(10)},
		{"sum precise", `sum([9007199254740993, 1])`, cty.NumberIntVal(9007199254740994)},

		// min/max
		{"min", `min(5, 2, 8)`, cty.NumberIntVal(2)},
		{"max", `max(5, 2, 8)`, cty.NumberIntVal(8)},

		// matchkeys
		{"matchkeys", `matchkeys(["a", "b", "c"], ["x", "y", "x"], ["x"])`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("c"),
		})},

		// transpose
		{"transpose", `transpose({a = ["1"], b = ["1", "2"]})`, cty.MapVal(map[string]cty.Value{
			"1": cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
			"2": cty.ListVal([]cty.Value{cty.StringVal("b")}),
		})},

		// setproduct
		{"setproduct length", `length(setproduct(["a", "b"], ["1", "2"]))`, cty.NumberIntVal(4)},

		// setintersection
		{"setintersection", `setintersection(["a", "b"], ["b", "c"])`, cty.SetVal([]cty.Value{
			cty.StringVal("b"),
		})},

		// setunion
		{"setunion", `sort(tolist(setunion(["a", "b"], ["b", "c"])))`, cty.ListVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c"),
		})},

		// setsubtract
		{"setsubtract", `setsubtract(["a", "b", "c"], ["b"])`, cty.SetVal([]cty.Value{
			cty.StringVal("a"), cty.StringVal("c"),
		})},

		// alltrue/anytrue
		{"alltrue", `alltrue([true, true, true])`, cty.BoolVal(true)},
		{"alltrue false", `alltrue([true, false])`, cty.BoolVal(false)},
		{"anytrue", `anytrue([false, true, false])`, cty.BoolVal(true)},
		{"anytrue false", `anytrue([false, false])`, cty.BoolVal(false)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestNumericFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"abs positive", `abs(5)`, cty.NumberIntVal(5)},
		{"abs negative", `abs(-5)`, cty.NumberIntVal(5)},
		{"ceil", `ceil(4.3)`, cty.NumberIntVal(5)},
		{"floor", `floor(4.7)`, cty.NumberIntVal(4)},
		{"signum positive", `signum(5)`, cty.NumberIntVal(1)},
		{"signum negative", `signum(-5)`, cty.NumberIntVal(-1)},
		{"signum zero", `signum(0)`, cty.NumberIntVal(0)},
		{"parseint", `parseint("42", 10)`, cty.NumberIntVal(42)},
		{"parseint hex", `parseint("ff", 16)`, cty.NumberIntVal(255)},
		{"pow", `pow(2, 3)`, cty.NumberIntVal(8)},
		{"log base 10", `floor(log(100, 10))`, cty.NumberIntVal(2)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestEncodingFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"base64encode", `base64encode("hello")`, cty.StringVal("aGVsbG8=")},
		{"base64decode", `base64decode("aGVsbG8=")`, cty.StringVal("hello")},
		{"jsonencode map", `jsonencode({a = 1})`, cty.StringVal(`{"a":1}`)},
		{"jsondecode", `jsondecode("{\"a\":1}").a`, cty.NumberIntVal(1)},
		// urlencode applies query-string encoding: spaces become + and non-ASCII
		// characters are percent-encoded as their UTF-8 bytes.
		{"urlencode", `urlencode("hello world")`, cty.StringVal("hello+world")},
		{"urlencode unicode", `urlencode("café")`, cty.StringVal("caf%C3%A9")},
		// base64gzip - check it returns non-empty string
		{"base64gzip", `base64gzip("hello") != ""`, cty.BoolVal(true)},
		// base64gzip pins OpenTofu's exact byte-for-byte output: OpenTofu flushes
		// the gzip writer before closing, emitting an empty sync-flush block.
		{
			"base64gzip exact",
			`base64gzip("hello world this is a test string")`,
			cty.StringVal("H4sIAAAAAAAA/8pIzcnJVyjPL8pJUSjJyCxWyCxWSFQoSS0uUSguKcrMSwcAAAD//wEAAP//d4eoGCEAAAA="),
		},
		// base64gunzip is the inverse of base64gzip, so the round trip is identity.
		{"base64gunzip roundtrip", `base64gunzip(base64gzip("hello"))`, cty.StringVal("hello")},
		// urldecode reverses urlencode; QueryUnescape also maps "+" to a space.
		{"urldecode percent", `urldecode("a%20b%26c")`, cty.StringVal("a b&c")},
		{"urldecode plus", `urldecode("x+y")`, cty.StringVal("x y")},
		{"urldecode roundtrip", `urldecode(urlencode("café / a b"))`, cty.StringVal("café / a b")},
		{"csvdecode length", `length(csvdecode("a,b\n1,2\n3,4"))`, cty.NumberIntVal(2)},
		{"textencodebase64", `textencodebase64("hello", "UTF-8")`, cty.StringVal("aGVsbG8=")},
		{"textdecodebase64", `textdecodebase64("aGVsbG8=", "UTF-8")`, cty.StringVal("hello")},
		// The encoding argument is honored: UTF-16LE encodes each ASCII
		// character as two bytes, so its base64 output differs from UTF-8.
		{"textencodebase64 utf16le", `textencodebase64("hello", "UTF-16LE")`, cty.StringVal("aABlAGwAbABvAA==")},
		{"textdecodebase64 utf16le", `textdecodebase64("aABlAGwAbABvAA==", "UTF-16LE")`, cty.StringVal("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestBase64DecodeUTF8Validation(t *testing.T) {
	t.Parallel()
	fn := Functions("/tmp")["base64decode"]

	t.Run("valid utf-8", func(t *testing.T) {
		t.Parallel()
		result, err := fn.Call([]cty.Value{cty.StringVal("SGVsbG8=")})
		require.NoError(t, err)
		assert.Equal(t, cty.StringVal("Hello"), result)
	})

	tests := []struct {
		name  string
		input string
	}{
		{"single 0xff byte", "/w=="},
		{"single 0x80 byte", "gA=="},
		{"multiple invalid bytes", "//79"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := fn.Call([]cty.Value{cty.StringVal(tt.input)})
			require.Error(t, err)
			assert.Equal(t, "the result of decoding the provided string is not valid UTF-8", err.Error())
		})
	}
}

func TestHashFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"md5", `md5("hello")`, cty.StringVal("5d41402abc4b2a76b9719d911017c592")},
		{"sha256", `sha256("hello")`, cty.StringVal("2cf24dba5fb0a30e26e83b2ac5b9e29e1b161e5c1fa7425e73043362938b9824")},
		{"sha512", `substr(sha512("hello"), 0, 32)`, cty.StringVal("9b71d224bd62f3785d96d46ad3ea3d73")},
		{"sha1", `sha1("hello")`, cty.StringVal("aaf4c61ddcc5e8a2dabede0f3b482cd9aea9434d")},
		{"bcrypt starts", `substr(bcrypt("password"), 0, 4)`, cty.StringVal("$2a$")},
		{"base64sha256", `base64sha256("hello")`, cty.StringVal("LPJNul+wow4m6DsqxbninhsWHlwfp0JecwQzYpOLmCQ=")},
		// base64sha512 - check it returns non-empty string
		{"base64sha512", `base64sha512("hello") != ""`, cty.BoolVal(true)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestRsaDecrypt(t *testing.T) {
	t.Parallel()
	// Generate a test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Test message
	message := "secret message"

	// Encrypt the message with the public key
	//nolint:staticcheck // SA1019: Using deprecated function for Terraform compatibility testing
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, []byte(message))
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Base64 encode the ciphertext
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	// Encode private key as PEM
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(privateKey),
	})

	// Build the expression
	// Need to escape the PEM newlines for HCL
	pemStr := string(privateKeyPEM)

	// Create HCL context with functions
	ctx := &hcl.EvalContext{
		Functions: Functions("/tmp"),
		Variables: map[string]cty.Value{
			"ciphertext": cty.StringVal(ciphertextB64),
			"privatekey": cty.StringVal(pemStr),
		},
	}

	// Parse and evaluate
	expr, diags := hclsyntax.ParseExpression([]byte(`rsadecrypt(ciphertext, privatekey)`), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("Failed to parse expression: %s", diags.Error())
	}

	result, diags := expr.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("Failed to evaluate rsadecrypt: %s", diags.Error())
	}

	if result.AsString() != message {
		t.Errorf("Expected %q, got %q", message, result.AsString())
	}
}

func TestRsaDecryptPKCS8(t *testing.T) {
	t.Parallel()
	// Generate a test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Test message
	message := "another secret"

	// Encrypt the message with the public key
	//nolint:staticcheck // SA1019: Using deprecated function for Terraform compatibility testing
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, []byte(message))
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}

	// Base64 encode the ciphertext
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	// Encode private key as PKCS8 PEM
	pkcs8Bytes, err := x509.MarshalPKCS8PrivateKey(privateKey)
	if err != nil {
		t.Fatalf("Failed to marshal PKCS8: %v", err)
	}
	privateKeyPEM := pem.EncodeToMemory(&pem.Block{
		Type:  "PRIVATE KEY",
		Bytes: pkcs8Bytes,
	})

	pemStr := string(privateKeyPEM)

	// Create HCL context with functions
	ctx := &hcl.EvalContext{
		Functions: Functions("/tmp"),
		Variables: map[string]cty.Value{
			"ciphertext": cty.StringVal(ciphertextB64),
			"privatekey": cty.StringVal(pemStr),
		},
	}

	// Parse and evaluate
	expr, diags := hclsyntax.ParseExpression([]byte(`rsadecrypt(ciphertext, privatekey)`), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("Failed to parse expression: %s", diags.Error())
	}

	result, diags := expr.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("Failed to evaluate rsadecrypt with PKCS8: %s", diags.Error())
	}

	if result.AsString() != message {
		t.Errorf("Expected %q, got %q", message, result.AsString())
	}
}

func TestRsaDecryptOpenSSH(t *testing.T) {
	t.Parallel()
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	message := "openssh secret"

	//nolint:staticcheck // SA1019: Using deprecated function for Terraform compatibility testing
	ciphertext, err := rsa.EncryptPKCS1v15(rand.Reader, &privateKey.PublicKey, []byte(message))
	if err != nil {
		t.Fatalf("Failed to encrypt: %v", err)
	}
	ciphertextB64 := base64.StdEncoding.EncodeToString(ciphertext)

	block, err := ssh.MarshalPrivateKey(privateKey, "")
	if err != nil {
		t.Fatalf("Failed to marshal OpenSSH key: %v", err)
	}
	pemStr := string(pem.EncodeToMemory(block))

	ctx := &hcl.EvalContext{
		Functions: Functions("/tmp"),
		Variables: map[string]cty.Value{
			"ciphertext": cty.StringVal(ciphertextB64),
			"privatekey": cty.StringVal(pemStr),
		},
	}

	expr, diags := hclsyntax.ParseExpression([]byte(`rsadecrypt(ciphertext, privatekey)`), "test.hcl", hcl.Pos{Line: 1, Column: 1})
	if diags.HasErrors() {
		t.Fatalf("Failed to parse expression: %s", diags.Error())
	}

	result, diags := expr.Value(ctx)
	if diags.HasErrors() {
		t.Fatalf("Failed to evaluate rsadecrypt with OpenSSH key: %s", diags.Error())
	}

	if result.AsString() != message {
		t.Errorf("Expected %q, got %q", message, result.AsString())
	}
}

func TestRsaDecryptErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name       string
		ciphertext string
		privatekey string
		errContain string
	}{
		{
			name:       "invalid base64",
			ciphertext: "not-valid-base64!!!",
			privatekey: "-----BEGIN RSA PRIVATE KEY-----\ntest\n-----END RSA PRIVATE KEY-----",
			errContain: "invalid base64",
		},
		{
			name:       "invalid pem",
			ciphertext: "aGVsbG8=",
			privatekey: "not a pem key",
			errContain: "invalid private key",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			ctx := &hcl.EvalContext{
				Functions: Functions("/tmp"),
				Variables: map[string]cty.Value{
					"ciphertext": cty.StringVal(tt.ciphertext),
					"privatekey": cty.StringVal(tt.privatekey),
				},
			}

			expr, _ := hclsyntax.ParseExpression([]byte(`rsadecrypt(ciphertext, privatekey)`), "test.hcl", hcl.Pos{Line: 1, Column: 1})
			_, diags := expr.Value(ctx)

			if !diags.HasErrors() {
				t.Error("Expected error but got none")
				return
			}

			errStr := diags.Error()
			if !contains(errStr, tt.errContain) {
				t.Errorf("Expected error containing %q, got %q", tt.errContain, errStr)
			}
		})
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 ||
		(len(s) > 0 && len(substr) > 0 && findSubstring(s, substr)))
}

func findSubstring(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}

// Ensure fmt is used
var _ = fmt.Sprintf

func TestTypeFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"tostring", `tostring(42)`, cty.StringVal("42")},
		{"tostring int beyond float64", `tostring(9007199254740993)`, cty.StringVal("9007199254740993")},
		{"tostring int beyond int64", `tostring(12345678901234567890)`, cty.StringVal("12345678901234567890")},
		{"tostring high-precision decimal", `tostring(123.456789012345678)`, cty.StringVal("123.456789012345678")},
		{"tonumber", `tonumber("42")`, cty.NumberIntVal(42)},
		{"tobool true", `tobool("true")`, cty.BoolVal(true)},
		{"tobool false", `tobool("false")`, cty.BoolVal(false)},
		{"tobool null", `tobool(null)`, cty.NullVal(cty.Bool)},
		{"tostring null", `tostring(null)`, cty.NullVal(cty.String)},
		{"tolist", `length(tolist(toset(["a", "b"])))`, cty.NumberIntVal(2)},
		{"toset", `length(toset(["a", "b", "a"]))`, cty.NumberIntVal(2)},
		{"tomap", `tomap({a = "x"}).a`, cty.StringVal("x")},
		{"try success", `try("hello", "default")`, cty.StringVal("hello")},
		{"can true", `can(tostring(42))`, cty.BoolVal(true)},
		{"type string", `type("hello")`, cty.StringVal("string")},
		{"type number", `type(42)`, cty.StringVal("number")},
		{"type bool", `type(true)`, cty.StringVal("bool")},
		{"nonsensitive", `nonsensitive("hello")`, cty.StringVal("hello")},
		{"nonsensitive of sensitive", `nonsensitive(sensitive("hello"))`, cty.StringVal("hello")},
		{"issensitive plain", `issensitive("hello")`, cty.False},
		{"issensitive marked", `issensitive(sensitive("hello"))`, cty.True},
		{"issensitive after nonsensitive", `issensitive(nonsensitive(sensitive("hello")))`, cty.False},
		{"issensitive propagates through split", `issensitive(split("-", sensitive("a-b"))[0])`, cty.True},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

// TestSensitiveFuncsNull pins that `sensitive`, `nonsensitive`, and
// `issensitive` accept a null argument like OpenTofu, whose parameters all
// declare AllowNull. The funcs are called directly because a null argument
// would otherwise be rejected by cty before the impl ever runs.
func TestSensitiveFuncsNull(t *testing.T) {
	t.Parallel()

	null := cty.NullVal(cty.String)

	sens, err := sensitiveFunc.Call([]cty.Value{null})
	require.NoError(t, err)
	assert.Equal(t, null.Mark(SensitiveMark), sens)

	nonsens, err := nonsensitiveFunc.Call([]cty.Value{sens})
	require.NoError(t, err)
	assert.Equal(t, null, nonsens)

	is, err := issensitiveFunc.Call([]cty.Value{null})
	require.NoError(t, err)
	assert.Equal(t, cty.False, is)
}

// TestIsSensitiveUnknown pins that `issensitive` of an unknown value returns an
// unknown bool rather than committing to an answer: the value's sensitivity is
// not finalized until the value itself is known. Matches OpenTofu.
func TestIsSensitiveUnknown(t *testing.T) {
	t.Parallel()

	is, err := issensitiveFunc.Call([]cty.Value{cty.UnknownVal(cty.String)})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.Bool), is)
}

// TestEphemeralAsNull is called directly because no HCL function constructs
// ephemeral values; only variable/output declarations apply the mark.
func TestEphemeralAsNull(t *testing.T) {
	t.Parallel()

	got, err := ephemeralasnullFunc.Call([]cty.Value{cty.StringVal("hunter2").Mark(EphemeralMark)})
	require.NoError(t, err)
	assert.Equal(t, cty.NullVal(cty.String), got)

	// Nested ephemeral parts null out individually; non-ephemeral marks survive.
	got, err = ephemeralasnullFunc.Call([]cty.Value{cty.ObjectVal(map[string]cty.Value{
		"open":   cty.StringVal("visible"),
		"secret": cty.StringVal("hunter2").Mark(EphemeralMark).Mark(SensitiveMark),
	})})
	require.NoError(t, err)
	assert.Equal(t, cty.ObjectVal(map[string]cty.Value{
		"open":   cty.StringVal("visible"),
		"secret": cty.NullVal(cty.String).Mark(SensitiveMark),
	}), got)

	// An unknown value's ephemerality is not yet finalized, so it stays
	// unknown rather than becoming null.
	got, err = ephemeralasnullFunc.Call([]cty.Value{cty.UnknownVal(cty.String).Mark(EphemeralMark)})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.String), got)

	got, err = ephemeralasnullFunc.Call([]cty.Value{cty.StringVal("plain")})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("plain"), got)
}

// TestToNumber pins that `tonumber` parses the whole string as a decimal
// number rather than truncating at the first non-digit character. A `%d`-style
// parse would stop at the '.' in "3.14" or the 'e' in "1e2" and silently return
// a truncated integer.
func TestToNumber(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"integer", `tonumber("42")`, cty.MustParseNumberVal("42")},
		{"fraction", `tonumber("3.14")`, cty.MustParseNumberVal("3.14")},
		{"exponent", `tonumber("1e2")`, cty.MustParseNumberVal("1e2")},
		{"negative fraction", `tonumber("-2.5")`, cty.MustParseNumberVal("-2.5")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()

			result := evalExpr(t, "/tmp", tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestReplaceInvalidRegex pins that `replace` with a slash-wrapped pattern that
// is not a valid regular expression errors, matching OpenTofu rather than
// silently treating the pattern as a literal substring.
func TestReplaceInvalidRegex(t *testing.T) {
	t.Parallel()

	_, err := replaceFunc.Call([]cty.Value{
		cty.StringVal("abc"), cty.StringVal("/(/"), cty.StringVal("Z"),
	})
	assert.EqualError(t, err, "error parsing regexp: missing closing ): `(`")
}

// TestToNumberError pins that `tonumber` of a string that is not a decimal
// number errors rather than returning a truncated value.
func TestToNumberError(t *testing.T) {
	t.Parallel()

	_, err := makeToFunc(cty.Number).Call([]cty.Value{cty.StringVal("12abc")})
	assert.EqualError(t, err,
		`cannot convert "12abc" to number; given string must be a decimal representation of a number`)
}

// TestCoalesceErrors pins that `coalesce` errors when every argument is skipped.
// Both null and empty-string arguments are skipped, so an all-null or
// all-empty-string call has no value to return and must fail rather than yield
// null or an empty string.
func TestCoalesceErrors(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name string
		args []cty.Value
	}{
		{"both null", []cty.Value{
			cty.NullVal(cty.DynamicPseudoType), cty.NullVal(cty.DynamicPseudoType),
		}},
		{"all empty string", []cty.Value{cty.StringVal(""), cty.StringVal("")}},
		{"null then empty string", []cty.Value{
			cty.NullVal(cty.String), cty.StringVal(""),
		}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			_, err := coalesceFunc.Call(tt.args)
			assert.EqualError(t, err, "no non-null, non-empty-string arguments")
		})
	}
}

// TestSumEmptyList pins that `sum` of an empty list errors, as Terraform does,
// rather than returning zero.
func TestSumEmptyList(t *testing.T) {
	t.Parallel()
	_, err := sumFunc.Call([]cty.Value{cty.ListValEmpty(cty.Number)})
	assert.EqualError(t, err, "cannot sum an empty list")
}

// TestSumNullElement pins that `sum` of a list containing a null element
// returns a graceful argument error rather than panicking inside the function
// implementation.
func TestSumNullElement(t *testing.T) {
	t.Parallel()
	_, err := sumFunc.Call([]cty.Value{cty.TupleVal([]cty.Value{
		cty.NumberIntVal(1), cty.NullVal(cty.Number), cty.NumberIntVal(3),
	})})
	assert.EqualError(t, err, "argument must be list, set, or tuple of number values")
}

// TestTransposeNull pins that `transpose` returns a graceful argument error,
// rather than panicking inside the function implementation, when a list value
// is null or a list contains a null string.
func TestTransposeNull(t *testing.T) {
	t.Parallel()

	nullList := cty.MapVal(map[string]cty.Value{
		"a": cty.NullVal(cty.List(cty.String)),
	})
	_, err := transposeFunc.Call([]cty.Value{nullList})
	assert.EqualError(t, err, `cannot use null list for ["a"]`)

	nullString := cty.MapVal(map[string]cty.Value{
		"a": cty.ListVal([]cty.Value{cty.StringVal("x"), cty.NullVal(cty.String)}),
	})
	_, err = transposeFunc.Call([]cty.Value{nullString})
	assert.EqualError(t, err, `cannot use null string for ["a"][1]`)
}

// TestTransposeUnknown pins that `transpose` returns an unknown value when its
// input is not wholly known.
func TestTransposeUnknown(t *testing.T) {
	t.Parallel()
	got, err := transposeFunc.Call([]cty.Value{
		cty.UnknownVal(cty.Map(cty.List(cty.String))),
	})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.Map(cty.List(cty.String))), got)
}

// TestOneUnknownLengthSet pins that `one` returns an unknown value, rather than
// erroring, when given a set whose length is not yet known. The set is known but
// contains an unknown element, so it could hold either one or two members once
// resolved. OpenTofu defers to an unknown result here; counting the elements at
// face value would wrongly report "more than one element".
func TestOneUnknownLengthSet(t *testing.T) {
	t.Parallel()
	set := cty.SetVal([]cty.Value{cty.UnknownVal(cty.String), cty.StringVal("fixed")})
	got, err := oneFunc.Call([]cty.Value{set})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.String), got)
}

// TestOneSingleElement pins that `one` returns the sole element of a one-element
// collection, and TestOneTooMany that it errors on more than one known element.
func TestOneSingleElement(t *testing.T) {
	t.Parallel()
	got, err := oneFunc.Call([]cty.Value{cty.SetVal([]cty.Value{cty.StringVal("solo")})})
	require.NoError(t, err)
	assert.Equal(t, cty.StringVal("solo"), got)

	_, err = oneFunc.Call([]cty.Value{cty.SetVal([]cty.Value{
		cty.StringVal("a"), cty.StringVal("b"),
	})})
	assert.EqualError(t, err, "list has more than one element")
}

// TestCidrHostOutOfRange pins that `cidrhost` errors when the host number does
// not fit within the prefix's host bits, as OpenTofu does, rather than silently
// overflowing into the network portion of the address.
func TestCidrHostOutOfRange(t *testing.T) {
	t.Parallel()
	_, err := cidrHostFunc.Call([]cty.Value{cty.StringVal("10.0.0.0/24"), cty.NumberIntVal(256)})
	assert.EqualError(t, err, "prefix of 24 does not accommodate a host numbered 256")
}

// TestCidrContainsAddressFamilyMismatch pins that `cidrcontains` errors, rather
// than silently returning false, when the prefix and the candidate address are
// of different address families, matching OpenTofu.
func TestCidrContainsAddressFamilyMismatch(t *testing.T) {
	t.Parallel()
	_, err := cidrContainsFunc.Call([]cty.Value{
		cty.StringVal("10.0.0.0/8"), cty.StringVal("2001:db8::1"),
	})
	assert.EqualError(t, err, "address family mismatch: 10.0.0.0/8 vs. 2001:db8::1")
}

// TestCidrNetmaskIPv6 pins that `cidrnetmask` rejects an IPv6 prefix, as
// OpenTofu does, rather than rendering the mask: a netmask is an IPv4-only
// concept.
func TestCidrNetmaskIPv6(t *testing.T) {
	t.Parallel()
	_, err := cidrNetmaskFunc.Call([]cty.Value{cty.StringVal("2001:db8::/32")})
	assert.EqualError(t, err, "IPv6 addresses cannot have a netmask: 2001:db8::/32")
}

// TestToSetToListPreserveEmptyElementType pins that `toset` / `tolist` over a
// typed-but-empty collection preserves the element type rather than collapsing
// to `dynamic`.
func TestToSetToListPreserveEmptyElementType(t *testing.T) {
	t.Parallel()
	t.Run("toset of empty list(string)", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, "/tmp", `toset(slice(tolist(["a"]), 0, 0))`)
		assert.Equal(t, cty.SetValEmpty(cty.String), got)
	})

	t.Run("tolist of empty set(string)", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, "/tmp", `tolist(setsubtract(toset(["a"]), toset(["a"])))`)
		assert.Equal(t, cty.ListValEmpty(cty.String), got)
	})
}

// TestToCollectionUnify pins that `tolist` / `toset` / `tomap` unify the
// element types of a tuple or object whose elements have differing types, as
// OpenTofu does, rather than panicking on the mismatch. Number, string, and
// bool all unify to string.
func TestToCollectionUnify(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"tolist mixed", `tolist([1, "a", true])`, cty.ListVal([]cty.Value{
			cty.StringVal("1"), cty.StringVal("a"), cty.StringVal("true"),
		})},
		{"toset mixed", `toset([3, "1", 2])`, cty.SetVal([]cty.Value{
			cty.StringVal("1"), cty.StringVal("2"), cty.StringVal("3"),
		})},
		{"tomap mixed", `tomap({ a = 1, b = "x", c = true })`, cty.MapVal(map[string]cty.Value{
			"a": cty.StringVal("1"), "b": cty.StringVal("x"), "c": cty.StringVal("true"),
		})},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			assert.Equal(t, tt.expected, result)
		})
	}
}

// TestMatchkeysTypeUnify pins that `matchkeys` unifies the element types of
// `keys` and `searchset` before comparing, as OpenTofu does, so a numeric key
// matches a string search-set element rather than being missed.
func TestMatchkeysTypeUnify(t *testing.T) {
	t.Parallel()
	got := evalExpr(t, "/tmp", `matchkeys(["a", "b", "c"], [1, 2, 3], ["2"])`)
	assert.Equal(t, cty.ListVal([]cty.Value{cty.StringVal("b")}), got)
}

// TestMatchkeysLengthMismatch pins that `matchkeys` errors when `keys` and
// `values` have different lengths, as OpenTofu does, rather than silently
// zipping to the shorter of the two.
func TestMatchkeysLengthMismatch(t *testing.T) {
	t.Parallel()
	_, err := matchkeysFunc.Call([]cty.Value{
		cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
		cty.ListVal([]cty.Value{cty.StringVal("x")}),
		cty.ListVal([]cty.Value{cty.StringVal("x")}),
	})
	assert.EqualError(t, err, "length of keys and values should be equal")
}

func TestMatchkeysUnknownSearchset(t *testing.T) {
	t.Parallel()
	got, err := matchkeysFunc.Call([]cty.Value{
		cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}),
		cty.ListVal([]cty.Value{cty.StringVal("x"), cty.StringVal("y")}),
		cty.ListVal([]cty.Value{cty.UnknownVal(cty.String)}),
	})
	require.NoError(t, err)
	assert.Equal(t, cty.UnknownVal(cty.List(cty.String)), got)
}

func TestDateTimeFunctions(t *testing.T) {
	t.Parallel()

	is := func(v cty.Value) func(cty.Value) bool {
		return func(o cty.Value) bool {
			return v.Equals(o).True()
		}
	}

	tests := []struct {
		name  string
		expr  string
		check func(cty.Value) bool
	}{
		{
			"timestamp format",
			`timestamp()`,
			func(v cty.Value) bool {
				// Should be RFC3339 format
				s := v.AsString()
				return len(s) > 0 && s[4] == '-' && s[10] == 'T'
			},
		},
		{
			"plantimestamp format",
			`plantimestamp()`,
			func(v cty.Value) bool {
				// Resolves to the current time in RFC3339 format.
				s := v.AsString()
				return len(s) > 0 && s[4] == '-' && s[10] == 'T'
			},
		},
		{
			"timeadd",
			`timeadd("2023-01-01T00:00:00Z", "24h")`,
			is(cty.StringVal("2023-01-02T00:00:00Z")),
		},
		{
			"timecmp equal",
			`timecmp("2023-01-01T00:00:00Z", "2023-01-01T00:00:00Z")`,
			is(cty.NumberIntVal(0)),
		},
		{
			"timecmp less",
			`timecmp("2023-01-01T00:00:00Z", "2023-01-02T00:00:00Z")`,
			is(cty.NumberIntVal(-1)),
		},
		{
			"formatdate",
			`formatdate("YYYY-MM-DD", "2023-06-15T12:30:00Z")`,
			is(cty.StringVal("2023-06-15")),
		},
		{
			// Lowercase h/hh is the 24-hour clock.
			"formatdate 24-hour",
			`formatdate("hh:mm:ss", "2023-06-15T13:05:07Z")`,
			is(cty.StringVal("13:05:07")),
		},
		{
			// Uppercase H/HH is the 12-hour clock.
			"formatdate 12-hour",
			`formatdate("HH AA", "2023-06-15T13:05:07Z")`,
			is(cty.StringVal("01 PM")),
		},
		{
			// ZZZZZ keeps the colon, ZZZZ drops it, and 'at' is a literal.
			"formatdate timezone and literal",
			`formatdate("ZZZZZ 'at' ZZZZ", "2023-06-15T13:05:07Z")`,
			is(cty.StringVal("+00:00 at +0000")),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !tt.check(result) {
				t.Errorf("Check failed for %s, got %s", tt.name, result.GoString())
			}
		})
	}
}

func TestUUIDFunction(t *testing.T) {
	t.Parallel()
	result := evalExpr(t, "/tmp", `uuid()`)
	s := result.AsString()

	// UUID format: 8-4-4-4-12
	if len(s) != 36 {
		t.Errorf("Expected UUID length 36, got %d: %s", len(s), s)
	}
	if s[8] != '-' || s[13] != '-' || s[18] != '-' || s[23] != '-' {
		t.Errorf("Invalid UUID format: %s", s)
	}
}

func TestUUIDV5Function(t *testing.T) {
	t.Parallel()
	result := evalExpr(t, "/tmp", `uuidv5("dns", "example.com")`)
	expected := cty.StringVal("cfbff0d1-9375-5685-968c-48ce8b15ae17")

	if !result.RawEquals(expected) {
		t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
	}
}

func TestFileFunctions(t *testing.T) {
	t.Parallel()
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `file("test.txt")`)
		expected := cty.StringVal("hello world")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("filebase64", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `filebase64("test.txt")`)
		expected := cty.StringVal("aGVsbG8gd29ybGQ=")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("fileexists true", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `fileexists("test.txt")`)
		expected := cty.BoolVal(true)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("fileexists false", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `fileexists("nonexistent.txt")`)
		expected := cty.BoolVal(false)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create JSON file
	jsonFile := filepath.Join(tmpDir, "data.json")
	if err := os.WriteFile(jsonFile, []byte(`{"name": "test", "count": 42}`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("jsondecode file", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `jsondecode(file("data.json")).name`)
		expected := cty.StringVal("test")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create template file
	tmplFile := filepath.Join(tmpDir, "greeting.tpl")
	if err := os.WriteFile(tmplFile, []byte("Hello, ${name}!"), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("templatefile", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `templatefile("greeting.tpl", {name = "World"})`)
		expected := cty.StringVal("Hello, World!")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create a template file exercising a function call and a `for` directive,
	// which a naive ${var} substitution cannot render.
	richTmpl := filepath.Join(tmpDir, "rich.tpl")
	if err := os.WriteFile(richTmpl,
		[]byte(`Hello ${upper(name)}, you have %{ for n in nums }${n} %{ endfor }items`), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Run("templatefile renders functions and directives", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `templatefile("rich.tpl", {name = "ada", nums = [1, 2, 3]})`)
		expected := cty.StringVal("Hello ADA, you have 1 2 3 items")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.json"} {
		if err := os.WriteFile(filepath.Join(subDir, name), []byte("content"), 0o644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("fileset", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, tmpDir, `length(fileset("subdir", "*.txt"))`)
		expected := cty.NumberIntVal(2)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})
}

// TestNestedTemplateFunctions pins that a template rendered by templatefile may
// itself call templatestring and templatefile, as OpenTofu allows, and that a
// self-recursive templatefile stops at the recursion depth limit. No t.Parallel:
// the recursion-limit subtest uses t.Setenv.
func TestNestedTemplateFunctions(t *testing.T) {
	tmpDir := t.TempDir()
	write := func(name, content string) {
		require.NoError(t, os.WriteFile(filepath.Join(tmpDir, name), []byte(content), 0o644))
	}
	write("string.tpl", `${templatestring("hi $${name}", { name = "bob" })}`)
	write("chain.tpl", `${templatefile("inner.tpl", { name = "eve" })}`)
	write("inner.tpl", `hello ${name}`)
	write("loop.tpl", `${templatefile("loop.tpl", {})}`)

	t.Run("templatestring inside templatefile", func(t *testing.T) {
		assert.Equal(t, cty.StringVal("hi bob"), evalExpr(t, tmpDir, `templatefile("string.tpl", {})`))
	})

	t.Run("templatefile inside templatefile", func(t *testing.T) {
		assert.Equal(t, cty.StringVal("hello eve"), evalExpr(t, tmpDir, `templatefile("chain.tpl", {})`))
	})

	t.Run("recursion limit", func(t *testing.T) {
		t.Setenv("TF_TEMPLATE_RECURSION_DEPTH", "5")
		expr, diags := hclsyntax.ParseExpression(
			[]byte(`templatefile("loop.tpl", {})`), "test.hcl", hcl.Pos{Line: 1, Column: 1})
		require.False(t, diags.HasErrors(), diags.Error())
		_, diags = expr.Value(&hcl.EvalContext{Functions: Functions(tmpDir)})
		require.True(t, diags.HasErrors())
		assert.Contains(t, diags.Error(), "maximum recursion depth 5 reached")
	})
}

// TestFileTildeExpansion pins that the file-reading functions expand a leading
// `~` to the user's home directory, matching OpenTofu's openFile. Before the
// fix these functions treated `~` as a literal path segment relative to baseDir
// and so failed to find a file OpenTofu reads successfully.
func TestFileTildeExpansion(t *testing.T) {
	t.Parallel()
	home, err := os.UserHomeDir()
	require.NoError(t, err)

	// Write a uniquely named file directly in the home directory so the `~`
	// path resolves to it; remove it afterwards.
	name := fmt.Sprintf(".pulumi_hcl_tilde_test_%d.txt", os.Getpid())
	abs := filepath.Join(home, name)
	require.NoError(t, os.WriteFile(abs, []byte("hi"), 0o600))
	t.Cleanup(func() { _ = os.Remove(abs) })

	// baseDir deliberately differs from home so a missing `~` expansion would
	// resolve the path under baseDir and fail.
	baseDir := t.TempDir()
	tildePath := fmt.Sprintf("~/%s", name)

	t.Run("file", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, baseDir, fmt.Sprintf("file(%q)", tildePath))
		assert.Equal(t, cty.StringVal("hi"), got)
	})

	t.Run("filebase64", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, baseDir, fmt.Sprintf("filebase64(%q)", tildePath))
		assert.Equal(t, cty.StringVal(base64.StdEncoding.EncodeToString([]byte("hi"))), got)
	})

	t.Run("fileexists", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, baseDir, fmt.Sprintf("fileexists(%q)", tildePath))
		assert.Equal(t, cty.True, got)
	})

	t.Run("filemd5", func(t *testing.T) {
		t.Parallel()
		got := evalExpr(t, baseDir, fmt.Sprintf("filemd5(%q)", tildePath))
		assert.Equal(t, evalExpr(t, baseDir, `md5("hi")`), got)
	})
}

// TestFileExistsDirectory pins that fileexists errors on a path that exists but
// is a directory rather than a regular file, matching OpenTofu. This is an
// error path, so it calls the function directly rather than via evalExpr.
func TestFileExistsDirectory(t *testing.T) {
	t.Parallel()
	baseDir := t.TempDir()
	dir := filepath.Join(baseDir, "adir")
	require.NoError(t, os.Mkdir(dir, 0o755))

	_, err := fileExistsFunc(baseDir).Call([]cty.Value{cty.StringVal("adir")})
	require.EqualError(t, err, fmt.Sprintf("%s is a directory, not a file", dir))
}

// TestFilesetGlob exercises fileset's glob semantics against OpenTofu: the `**`
// operator recurses, directories are excluded, and paths use forward slashes.
func TestFilesetGlob(t *testing.T) {
	t.Parallel()
	root := t.TempDir()

	for _, p := range []string{
		"top.txt",
		"nested/mid.txt",
		"nested/deep/low.txt",
		"nested/note.md",
	} {
		full := filepath.Join(root, filepath.FromSlash(p))
		require.NoError(t, os.MkdirAll(filepath.Dir(full), 0o755))
		require.NoError(t, os.WriteFile(full, []byte("x"), 0o644))
	}

	t.Run("recursive matches files at every depth", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, root, `fileset(".", "**")`)
		assert.Equal(t, cty.SetVal([]cty.Value{
			cty.StringVal("nested/deep/low.txt"),
			cty.StringVal("nested/mid.txt"),
			cty.StringVal("nested/note.md"),
			cty.StringVal("top.txt"),
		}), result)
	})

	t.Run("recursive with extension filter", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, root, `fileset(".", "**/*.txt")`)
		assert.Equal(t, cty.SetVal([]cty.Value{
			cty.StringVal("nested/deep/low.txt"),
			cty.StringVal("nested/mid.txt"),
			cty.StringVal("top.txt"),
		}), result)
	})

	t.Run("single star excludes directories", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, root, `fileset(".", "*")`)
		assert.Equal(t, cty.SetVal([]cty.Value{
			cty.StringVal("top.txt"),
		}), result)
	})
}

func TestIPFunctions(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"cidrcontains ip in", `cidrcontains("10.0.0.0/8", "10.5.6.7")`, cty.True},
		{"cidrcontains ip out", `cidrcontains("10.0.0.0/8", "192.168.1.1")`, cty.False},
		{"cidrcontains prefix in", `cidrcontains("10.0.0.0/8", "10.1.0.0/16")`, cty.True},
		{"cidrcontains prefix out", `cidrcontains("10.0.0.0/16", "10.1.0.0/16")`, cty.False},
		{"cidrcontains v6 in", `cidrcontains("2001:db8::/32", "2001:db8:1::1")`, cty.True},
		{"cidrhost", `cidrhost("10.0.0.0/8", 5)`, cty.StringVal("10.0.0.5")},
		{"cidrhost neg one", `cidrhost("10.0.0.0/24", -1)`, cty.StringVal("10.0.0.255")},
		{"cidrhost neg two", `cidrhost("10.0.0.0/24", -2)`, cty.StringVal("10.0.0.254")},
		{"cidrhost leading zeros", `cidrhost("010.001.0.0/24", 5)`, cty.StringVal("10.1.0.5")},
		{"cidrnetmask", `cidrnetmask("10.0.0.0/8")`, cty.StringVal("255.0.0.0")},
		{"cidrnetmask leading zeros", `cidrnetmask("010.001.0.0/24")`, cty.StringVal("255.255.255.0")},
		{"cidrsubnet v4", `cidrsubnet("10.0.0.0/8", 8, 2)`, cty.StringVal("10.2.0.0/16")},
		{"cidrsubnet leading zeros", `cidrsubnet("010.001.0.0/16", 8, 2)`, cty.StringVal("10.1.2.0/24")},
		{
			"cidrsubnets leading zeros", `cidrsubnets("010.001.0.0/16", 8, 8)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("10.1.0.0/24"),
				cty.StringVal("10.1.1.0/24"),
			}),
		},
		{
			"cidrsubnet v6", `cidrsubnet("2600:1f14:315a:f400::/56", 8, 2)`,
			cty.StringVal("2600:1f14:315a:f402::/64"),
		},
		{"cidrsubnets count", `length(cidrsubnets("10.0.0.0/8", 8, 8, 8))`, cty.NumberIntVal(3)},
		{
			"cidrsubnets v4 values", `cidrsubnets("10.0.0.0/8", 8, 8, 8)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("10.0.0.0/16"),
				cty.StringVal("10.1.0.0/16"),
				cty.StringVal("10.2.0.0/16"),
			}),
		},
		{
			"cidrsubnets v4 mixed bits", `cidrsubnets("10.0.0.0/8", 8, 4, 4)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("10.0.0.0/16"),
				cty.StringVal("10.16.0.0/12"),
				cty.StringVal("10.32.0.0/12"),
			}),
		},
		{
			"cidrsubnets v6 values", `cidrsubnets("2600:1f14:315a:f400::/56", 8, 8, 8)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("2600:1f14:315a:f400::/64"),
				cty.StringVal("2600:1f14:315a:f401::/64"),
				cty.StringVal("2600:1f14:315a:f402::/64"),
			}),
		},
		{
			"cidrsubnets v6 uneven bits", `cidrsubnets("2600:1f14:315a::/48", 16, 16, 16)`,
			cty.ListVal([]cty.Value{
				cty.StringVal("2600:1f14:315a::/64"),
				cty.StringVal("2600:1f14:315a:1::/64"),
				cty.StringVal("2600:1f14:315a:2::/64"),
			}),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestYAMLFunctions(t *testing.T) {
	t.Parallel()
	t.Run("yamlencode", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `yamlencode({a = "x", b = 2, c = [1, 2]})`)
		assert.Equal(t, "\"a\": \"x\"\n\"b\": 2\n\"c\":\n- 1\n- 2\n", result.AsString())
	})

	t.Run("yamldecode", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `yamldecode("a: 1\nb: 2\n").a`)
		assert.True(t, result.RawEquals(cty.NumberIntVal(1)), "got %s", result.GoString())
	})

	t.Run("yamldecode timestamp", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `yamldecode("d: 2020-01-01").d`)
		assert.Equal(t, cty.StringVal("2020-01-01T00:00:00Z"), result)
	})
}

// Regression for https://github.com/pulumi-labs/pulumi-hcl/issues/143:
// functions that recurse into argument collections must lift nested unknowns
// to an unknown result, not panic when ctyToGo or similar helpers reach an
// unknown leaf. go-cty's AllowUnknown:false only auto-lifts shallow unknowns,
// so each function below is responsible for the deep check itself.
func TestUnknownPropagation(t *testing.T) {
	t.Parallel()
	funcs := Functions("/tmp")

	tests := []struct {
		name string
		fn   string
		args []cty.Value
		want cty.Value
	}{
		{
			name: "yamlencode with unknown nested in object",
			fn:   "yamlencode",
			args: []cty.Value{cty.ObjectVal(map[string]cty.Value{"id": cty.UnknownVal(cty.String)})},
			want: cty.UnknownVal(cty.String),
		},
		{
			name: "tostring with unknown nested in object",
			fn:   "tostring",
			args: []cty.Value{cty.ObjectVal(map[string]cty.Value{"id": cty.UnknownVal(cty.String)})},
			want: cty.UnknownVal(cty.String),
		},
		{
			name: "alltrue with unknown element",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.True, cty.UnknownVal(cty.Bool)})},
			want: cty.UnknownVal(cty.Bool),
		},
		{
			name: "anytrue with unknown element (no true present)",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.False, cty.UnknownVal(cty.Bool)})},
			want: cty.UnknownVal(cty.Bool),
		},
		{
			name: "index with unknown element in haystack",
			fn:   "index",
			args: []cty.Value{
				cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)}),
				cty.StringVal("b"),
			},
			want: cty.UnknownVal(cty.Number),
		},
		{
			name: "index with unknown element after goal",
			fn:   "index",
			args: []cty.Value{
				cty.ListVal([]cty.Value{cty.StringVal("a"), cty.UnknownVal(cty.String)}),
				cty.StringVal("a"),
			},
			want: cty.NumberIntVal(0),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, ok := funcs[tt.fn]
			require.True(t, ok, "function %q not registered", tt.fn)

			got, err := fn.Call(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAnyTrueAllTrue(t *testing.T) {
	t.Parallel()
	funcs := Functions("/tmp")

	unknownBool := cty.UnknownVal(cty.Bool)
	nullBool := cty.NullVal(cty.Bool)

	tests := []struct {
		name string
		fn   string
		args []cty.Value
		want cty.Value
	}{
		{
			name: "anytrue known true after unknown",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool, cty.True})},
			want: cty.True,
		},
		{
			name: "anytrue known true before unknown",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.True, unknownBool})},
			want: cty.True,
		},
		{
			name: "anytrue unknown and false",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool, cty.False})},
			want: unknownBool,
		},
		{
			name: "anytrue only unknown",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool})},
			want: unknownBool,
		},
		{
			name: "anytrue unknown list",
			fn:   "anytrue",
			args: []cty.Value{cty.UnknownVal(cty.List(cty.Bool))},
			want: unknownBool,
		},
		{
			name: "anytrue all false",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.False, cty.False})},
			want: cty.False,
		},
		{
			name: "anytrue empty list",
			fn:   "anytrue",
			args: []cty.Value{cty.ListValEmpty(cty.Bool)},
			want: cty.False,
		},
		{
			name: "anytrue only null",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{nullBool})},
			want: cty.False,
		},
		{
			name: "anytrue null then true",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{nullBool, cty.True})},
			want: cty.True,
		},
		{
			name: "anytrue null and unknown",
			fn:   "anytrue",
			args: []cty.Value{cty.ListVal([]cty.Value{nullBool, unknownBool})},
			want: unknownBool,
		},
		{
			name: "alltrue unknown before false",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool, cty.False})},
			want: unknownBool,
		},
		{
			name: "alltrue known false before unknown",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.False, unknownBool})},
			want: cty.False,
		},
		{
			name: "alltrue unknown and true",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool, cty.True})},
			want: unknownBool,
		},
		{
			name: "alltrue only unknown",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{unknownBool})},
			want: unknownBool,
		},
		{
			name: "alltrue unknown list",
			fn:   "alltrue",
			args: []cty.Value{cty.UnknownVal(cty.List(cty.Bool))},
			want: unknownBool,
		},
		{
			name: "alltrue all true",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.True, cty.True})},
			want: cty.True,
		},
		{
			name: "alltrue empty list",
			fn:   "alltrue",
			args: []cty.Value{cty.ListValEmpty(cty.Bool)},
			want: cty.True,
		},
		{
			name: "alltrue only null",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{nullBool})},
			want: cty.False,
		},
		{
			name: "alltrue true then null",
			fn:   "alltrue",
			args: []cty.Value{cty.ListVal([]cty.Value{cty.True, nullBool})},
			want: cty.False,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			fn, ok := funcs[tt.fn]
			require.True(t, ok, "function %q not registered", tt.fn)

			got, err := fn.Call(tt.args)
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

// TestLengthMarkPropagation mirrors OpenTofu's LengthFunc, which carries the
// argument's marks onto the returned count for every supported type. The tuple
// and object branches build the count themselves, so they must reapply the
// marks; the string/list/map branches delegate to helpers that already
// preserve them.
func TestLengthMarkPropagation(t *testing.T) {
	t.Parallel()
	funcs := Functions("/tmp")
	length := funcs["length"]

	tests := []struct {
		name string
		arg  cty.Value
		want cty.Value
	}{
		{
			name: "tuple",
			arg:  cty.TupleVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b"), cty.StringVal("c")}).Mark(SensitiveMark),
			want: cty.NumberIntVal(3).Mark(SensitiveMark),
		},
		{
			name: "object",
			arg:  cty.ObjectVal(map[string]cty.Value{"a": cty.NumberIntVal(1), "b": cty.NumberIntVal(2)}).Mark(SensitiveMark),
			want: cty.NumberIntVal(2).Mark(SensitiveMark),
		},
		{
			name: "string",
			arg:  cty.StringVal("hello").Mark(SensitiveMark),
			want: cty.NumberIntVal(5).Mark(SensitiveMark),
		},
		{
			name: "list",
			arg:  cty.ListVal([]cty.Value{cty.StringVal("a"), cty.StringVal("b")}).Mark(SensitiveMark),
			want: cty.NumberIntVal(2).Mark(SensitiveMark),
		},
		{
			name: "map",
			arg:  cty.MapVal(map[string]cty.Value{"a": cty.StringVal("1")}).Mark(SensitiveMark),
			want: cty.NumberIntVal(1).Mark(SensitiveMark),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			got, err := length.Call([]cty.Value{tt.arg})
			require.NoError(t, err)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestAbspathAndBasename(t *testing.T) {
	t.Parallel()
	t.Run("basename", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `basename("/path/to/file.txt")`)
		expected := cty.StringVal("file.txt")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("dirname", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `dirname("/path/to/file.txt")`)
		expected := cty.StringVal("/path/to")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("pathexpand", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `pathexpand("~")`)
		// Should expand to home directory, not be empty
		if result.AsString() == "" || result.AsString() == "~" {
			t.Errorf("Expected home directory expansion, got %s", result.AsString())
		}
	})

	t.Run("abspath", func(t *testing.T) {
		t.Parallel()
		result := evalExpr(t, "/tmp", `abspath("test.txt")`)
		expected := cty.StringVal("/tmp/test.txt")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})
}
