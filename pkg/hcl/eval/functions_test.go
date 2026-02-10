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
	"github.com/zclconf/go-cty/cty"
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

func TestStringFunctions(t *testing.T) {
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

		// substr
		{"substr", `substr("hello", 0, 3)`, cty.StringVal("hel")},

		// format
		{"format basic", `format("Hello, %s!", "World")`, cty.StringVal("Hello, World!")},
		{"format number", `format("Count: %d", 42)`, cty.StringVal("Count: 42")},

		// chomp
		{"chomp", `chomp("hello\n")`, cty.StringVal("hello")},

		// indent
		{"indent", `indent(2, "hello\nworld")`, cty.StringVal("  hello\n  world")},

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
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestCollectionFunctions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		// length (stdlib.LengthFunc only works with collections, not strings)
		{"length list", `length(["a", "b", "c"])`, cty.NumberIntVal(3)},
		{"length tuple", `length(tolist(["a", "b"]))`, cty.NumberIntVal(2)},

		// element
		{"element", `element(["a", "b", "c"], 1)`, cty.StringVal("b")},
		{"element wrap", `element(["a", "b", "c"], 4)`, cty.StringVal("b")}, // wraps around

		// index
		{"index", `index(["a", "b", "c"], "b")`, cty.NumberIntVal(1)},

		// lookup
		{"lookup found", `lookup({a = "x", b = "y"}, "a", "default")`, cty.StringVal("x")},
		{"lookup default", `lookup({a = "x"}, "b", "default")`, cty.StringVal("default")},

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

		// coalesce (coalesce returns first non-empty, "" is technically a value so it's returned)
		{"coalesce", `coalesce("a", "b")`, cty.StringVal("a")},

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

		// sum
		{"sum", `sum([1, 2, 3, 4])`, cty.NumberIntVal(10)},

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
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestNumericFunctions(t *testing.T) {
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
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestEncodingFunctions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"base64encode", `base64encode("hello")`, cty.StringVal("aGVsbG8=")},
		{"base64decode", `base64decode("aGVsbG8=")`, cty.StringVal("hello")},
		{"jsonencode map", `jsonencode({a = 1})`, cty.StringVal(`{"a":1}`)},
		{"jsondecode", `jsondecode("{\"a\":1}").a`, cty.NumberIntVal(1)},
		// urlencode uses %20 for spaces (not + which is form-encoded)
		{"urlencode", `urlencode("hello world")`, cty.StringVal("hello%20world")},
		// base64gzip - check it returns non-empty string
		{"base64gzip", `base64gzip("hello") != ""`, cty.BoolVal(true)},
		{"csvdecode length", `length(csvdecode("a,b\n1,2\n3,4"))`, cty.NumberIntVal(2)},
		{"textencodebase64", `textencodebase64("hello", "UTF-8")`, cty.StringVal("aGVsbG8=")},
		{"textdecodebase64", `textdecodebase64("aGVsbG8=", "UTF-8")`, cty.StringVal("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestHashFunctions(t *testing.T) {
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
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestRsaDecrypt(t *testing.T) {
	// Generate a test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Test message
	message := "secret message"

	// Encrypt the message with the public key
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
	// Generate a test RSA key pair
	privateKey, err := rsa.GenerateKey(rand.Reader, 2048)
	if err != nil {
		t.Fatalf("Failed to generate RSA key: %v", err)
	}

	// Test message
	message := "another secret"

	// Encrypt the message with the public key
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

func TestRsaDecryptErrors(t *testing.T) {
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
			errContain: "invalid PEM",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
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
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"tostring", `tostring(42)`, cty.StringVal("42")},
		{"tonumber", `tonumber("42")`, cty.NumberIntVal(42)},
		{"tobool true", `tobool("true")`, cty.BoolVal(true)},
		{"tobool false", `tobool("false")`, cty.BoolVal(false)},
		{"tolist", `length(tolist(toset(["a", "b"])))`, cty.NumberIntVal(2)},
		{"toset", `length(toset(["a", "b", "a"]))`, cty.NumberIntVal(2)},
		{"tomap", `tomap({a = "x"}).a`, cty.StringVal("x")},
		{"try success", `try("hello", "default")`, cty.StringVal("hello")},
		{"can true", `can(tostring(42))`, cty.BoolVal(true)},
		{"type string", `type("hello")`, cty.StringVal("string")},
		{"type number", `type(42)`, cty.StringVal("number")},
		{"type bool", `type(true)`, cty.StringVal("bool")},
		{"nonsensitive", `nonsensitive("hello")`, cty.StringVal("hello")},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestDateTimeFunctions(t *testing.T) {
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
			"timeadd",
			`timeadd("2023-01-01T00:00:00Z", "24h")`,
			func(v cty.Value) bool {
				return v.AsString() == "2023-01-02T00:00:00Z"
			},
		},
		{
			"timecmp equal",
			`timecmp("2023-01-01T00:00:00Z", "2023-01-01T00:00:00Z")`,
			func(v cty.Value) bool {
				bf := v.AsBigFloat()
				i, _ := bf.Int64()
				return i == 0
			},
		},
		{
			"timecmp less",
			`timecmp("2023-01-01T00:00:00Z", "2023-01-02T00:00:00Z")`,
			func(v cty.Value) bool {
				bf := v.AsBigFloat()
				i, _ := bf.Int64()
				return i == -1
			},
		},
		{
			"formatdate",
			`formatdate("YYYY-MM-DD", "2023-06-15T12:30:00Z")`,
			func(v cty.Value) bool {
				return v.AsString() == "2023-06-15"
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalExpr(t, "/tmp", tt.expr)
			if !tt.check(result) {
				t.Errorf("Check failed for %s, got %s", tt.name, result.GoString())
			}
		})
	}
}

func TestUUIDFunction(t *testing.T) {
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
	result := evalExpr(t, "/tmp", `uuidv5("dns", "example.com")`)
	expected := cty.StringVal("cfbff0d1-9375-5685-968c-48ce8b15ae17")

	if !result.RawEquals(expected) {
		t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
	}
}

func TestFileFunctions(t *testing.T) {
	tmpDir := t.TempDir()

	// Create test file
	testFile := filepath.Join(tmpDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello world"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("file", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `file("test.txt")`)
		expected := cty.StringVal("hello world")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("filebase64", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `filebase64("test.txt")`)
		expected := cty.StringVal("aGVsbG8gd29ybGQ=")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("fileexists true", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `fileexists("test.txt")`)
		expected := cty.BoolVal(true)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("fileexists false", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `fileexists("nonexistent.txt")`)
		expected := cty.BoolVal(false)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create JSON file
	jsonFile := filepath.Join(tmpDir, "data.json")
	if err := os.WriteFile(jsonFile, []byte(`{"name": "test", "count": 42}`), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("jsondecode file", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `jsondecode(file("data.json")).name`)
		expected := cty.StringVal("test")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create template file
	tmplFile := filepath.Join(tmpDir, "greeting.tpl")
	if err := os.WriteFile(tmplFile, []byte("Hello, ${name}!"), 0644); err != nil {
		t.Fatal(err)
	}

	t.Run("templatefile", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `templatefile("greeting.tpl", {name = "World"})`)
		expected := cty.StringVal("Hello, World!")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	// Create subdirectory with files
	subDir := filepath.Join(tmpDir, "subdir")
	if err := os.Mkdir(subDir, 0755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"a.txt", "b.txt", "c.json"} {
		if err := os.WriteFile(filepath.Join(subDir, name), []byte("content"), 0644); err != nil {
			t.Fatal(err)
		}
	}

	t.Run("fileset", func(t *testing.T) {
		result := evalExpr(t, tmpDir, `length(fileset("subdir", "*.txt"))`)
		expected := cty.NumberIntVal(2)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})
}

func TestIPFunctions(t *testing.T) {
	tests := []struct {
		name     string
		expr     string
		expected cty.Value
	}{
		{"cidrhost", `cidrhost("10.0.0.0/8", 5)`, cty.StringVal("10.0.0.5")},
		{"cidrnetmask", `cidrnetmask("10.0.0.0/8")`, cty.StringVal("255.0.0.0")},
		{"cidrsubnet", `cidrsubnet("10.0.0.0/8", 8, 2)`, cty.StringVal("10.2.0.0/16")},
		{"cidrsubnets count", `length(cidrsubnets("10.0.0.0/8", 8, 8, 8))`, cty.NumberIntVal(3)},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result := evalExpr(t, "/tmp", tt.expr)
			if !result.RawEquals(tt.expected) {
				t.Errorf("Expected %s, got %s", tt.expected.GoString(), result.GoString())
			}
		})
	}
}

func TestYAMLFunctions(t *testing.T) {
	t.Run("yamlencode", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `yamlencode({a = "x", b = 2})`)
		s := result.AsString()
		// YAML encoding should produce valid output
		if len(s) == 0 {
			t.Error("Expected non-empty YAML output")
		}
	})

	t.Run("yamldecode", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `yamldecode("a: 1\nb: 2\n").a`)
		expected := cty.NumberIntVal(1)
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})
}

func TestAbspathAndBasename(t *testing.T) {
	t.Run("basename", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `basename("/path/to/file.txt")`)
		expected := cty.StringVal("file.txt")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("dirname", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `dirname("/path/to/file.txt")`)
		expected := cty.StringVal("/path/to")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})

	t.Run("pathexpand", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `pathexpand("~")`)
		// Should expand to home directory, not be empty
		if result.AsString() == "" || result.AsString() == "~" {
			t.Errorf("Expected home directory expansion, got %s", result.AsString())
		}
	})

	t.Run("abspath", func(t *testing.T) {
		result := evalExpr(t, "/tmp", `abspath("test.txt")`)
		expected := cty.StringVal("/tmp/test.txt")
		if !result.RawEquals(expected) {
			t.Errorf("Expected %s, got %s", expected.GoString(), result.GoString())
		}
	})
}
