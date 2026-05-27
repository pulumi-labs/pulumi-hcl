# `provider = simple` explicitly references the un-aliased default in
# this scope, which the parent has mapped to its `simple.configured`
# block via `providers = { simple = ... }`. Distinct from the implicit
# default case — exercises the `res.Provider != nil` pass-through path.
resource "simple_resource" "r" {
  provider  = simple
  input_one = "world"
  input_two = true
}

data "simple_lookup" "ds" {
  provider = simple
  query    = "world"
}

output "resource_prefix_result" {
  value = simple_resource.r.prefix_result
}

output "data_prefix_result" {
  value = data.simple_lookup.ds.prefix_result
}
