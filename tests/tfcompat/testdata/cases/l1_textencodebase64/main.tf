# Terraform's `textencodebase64` / `textdecodebase64` honor the named IANA
# character encoding: the string is transcoded into (or out of) that encoding
# around the base64 step. UTF-16LE encodes each ASCII character as two bytes, so
# its base64 output differs from the UTF-8 encoding of the same string.
output "utf16le" {
  value = textencodebase64("hello", "UTF-16LE")
}

output "utf8" {
  value = textencodebase64("hello", "UTF-8")
}

output "decode_utf16le" {
  value = textdecodebase64("aABlAGwAbABvAA==", "UTF-16LE")
}

output "roundtrip" {
  value = textdecodebase64(textencodebase64("café", "UTF-16LE"), "UTF-16LE")
}
