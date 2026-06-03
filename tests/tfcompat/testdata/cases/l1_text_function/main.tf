# Built-in functions exercised purely through their string/scalar outputs:
# indentation, query-string URL encoding/decoding, IANA-charset base64
# transcoding, gzip+base64 round trips, and the cidrcontains IP-containment
# predicate. Each output pins pulumi-hcl's result against OpenTofu's.

# `indent` adds the given number of spaces after every newline, leaving the
# first line untouched and still padding otherwise-blank lines. The surrounding
# brackets make the leading whitespace of each line visible in the output.
output "indent_two_lines" {
  value = "[${indent(2, "hello\nworld")}]"
}

output "indent_blank_line" {
  value = "[${indent(2, "a\n\nb")}]"
}

output "indent_list_block" {
  value = "items: ${indent(4, "[\n  a,\n  b,\n]")}"
}

output "indent_single_line" {
  value = "[${indent(4, "no newline")}]"
}

# `urlencode` applies query-string URL encoding (Go's url.QueryEscape): spaces
# become `+` and non-ASCII characters are first encoded as UTF-8 and then
# percent-encoded byte by byte.
output "urlencode_space" {
  value = urlencode("a b c")
}

output "urlencode_reserved" {
  value = urlencode("a/b?c=d&e")
}

output "urlencode_plus" {
  value = urlencode("a+b")
}

output "urlencode_unicode" {
  value = urlencode("café")
}

output "urlencode_emoji" {
  value = urlencode("hi 😀")
}

# `urldecode` is the inverse of `urlencode`: it reverses percent encoding and,
# like url.QueryUnescape, maps a literal `+` to a space.
output "urldecode_percent" {
  value = urldecode("a%20b%26c%3Dd")
}

output "urldecode_plus" {
  value = urldecode("x+y")
}

output "urldecode_roundtrip" {
  value = urldecode(urlencode("a b/c?d=e&f"))
}

# `textencodebase64` / `textdecodebase64` honor the named IANA character
# encoding: the string is transcoded into (or out of) that encoding around the
# base64 step. UTF-16LE encodes each ASCII character as two bytes, so its base64
# output differs from the UTF-8 encoding of the same string.
output "textbase64_utf16le" {
  value = textencodebase64("hello", "UTF-16LE")
}

output "textbase64_utf8" {
  value = textencodebase64("hello", "UTF-8")
}

output "textbase64_decode_utf16le" {
  value = textdecodebase64("aABlAGwAbABvAA==", "UTF-16LE")
}

output "textbase64_roundtrip" {
  value = textdecodebase64(textencodebase64("café", "UTF-16LE"), "UTF-16LE")
}

# `base64gunzip` is the inverse of `base64gzip`: it base64-decodes its argument
# and gunzips the result, so the round trip is the identity.
output "base64gunzip_roundtrip" {
  value = base64gunzip(base64gzip("Hello, OpenTofu!"))
}

# The raw `base64gzip` output is itself observable: OpenTofu flushes the gzip
# writer before closing it, which emits an empty sync-flush block, so the exact
# base64 string must match byte for byte rather than only round-tripping.
output "base64gzip_raw" {
  value = base64gzip("hello world this is a test string")
}

# `cidrcontains` reports whether the IP address or prefix in its second argument
# falls within the containing prefix in its first. An IP argument is contained
# when it lies inside the prefix; a prefix argument is contained only when both
# ends of its range do.
output "cidrcontains_ip_in" {
  value = cidrcontains("10.0.0.0/8", "10.5.6.7")
}

output "cidrcontains_ip_out" {
  value = cidrcontains("10.0.0.0/8", "192.168.1.1")
}

output "cidrcontains_prefix_in" {
  value = cidrcontains("10.0.0.0/8", "10.1.0.0/16")
}

output "cidrcontains_prefix_out" {
  value = cidrcontains("10.0.0.0/16", "10.1.0.0/16")
}

output "cidrcontains_v6_in" {
  value = cidrcontains("2001:db8::/32", "2001:db8:1::1")
}
