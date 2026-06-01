# OpenTofu's `yamlencode` delegates to the go-cty-yaml encoder, which quotes
# every mapping key and string scalar, keeps block sequences at the same
# indentation as their key, and renders booleans and nulls as bare tokens.
output "scalars" {
  value = yamlencode({ a = 1, b = "two", c = [1, 2, 3] })
}

output "nested" {
  value = yamlencode({ obj = { x = true, y = null }, list = ["p", "q"] })
}

output "top_level_list" {
  value = yamlencode(["one", "two", "three"])
}
