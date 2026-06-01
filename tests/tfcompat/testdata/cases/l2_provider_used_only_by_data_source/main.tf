provider "simple" {
  alias  = "data_only"
  prefix = "lookup"
}

data "simple_lookup" "ds" {
  provider = simple.data_only
  query    = "world"
}

output "lookup_prefix_result" {
  value = data.simple_lookup.ds.prefix_result
}
