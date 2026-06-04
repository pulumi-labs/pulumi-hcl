provider "simple" {
  prefix = "hello"
}

data "simple_lookup" "ds" {
  provider = simple
  query    = "world"
}

output "lookup_prefix_result" {
  value = data.simple_lookup.ds.prefix_result
}
