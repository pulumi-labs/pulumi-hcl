# OpenTofu binds `yamlencode` and `yamldecode` to the go-cty-yaml encoder and
# decoder. The encoder quotes every mapping key and string scalar, keeps block
# sequences at the same indentation as their key, and renders booleans and
# nulls as bare tokens. The decoder resolves a YAML `!!timestamp` to its
# canonical RFC 3339 string rather than dropping it.
output "encode_scalars" {
  value = yamlencode({ a = 1, b = "two", c = [1, 2, 3] })
}

output "encode_nested" {
  value = yamlencode({ obj = { x = true, y = null }, list = ["p", "q"] })
}

output "encode_top_level_list" {
  value = yamlencode(["one", "two", "three"])
}

output "decode_scalars" {
  value = yamldecode("a: 1\nb: two\nc:\n- 1\n- 2\n")
}

output "decode_timestamp" {
  value = yamldecode("d: 2020-01-01")
}

# YAML integer-tag resolution: a scalar containing underscores is not an integer
# and stays a string, while plain decimal, octal (0o) and hex (0x) scalars
# resolve to numbers.
output "decode_int_tags" {
  value = {
    underscore_dec = yamldecode("v: 1_000").v
    underscore_oct = yamldecode("v: 0o1_0").v
    underscore_hex = yamldecode("v: 0x1_f").v
    plain_hex      = yamldecode("v: 0x1f").v
    plain_oct      = yamldecode("v: 0o17").v
  }
}

output "roundtrip" {
  value = yamldecode(yamlencode({ a = 1, b = "two", nested = { c = true } }))
}
