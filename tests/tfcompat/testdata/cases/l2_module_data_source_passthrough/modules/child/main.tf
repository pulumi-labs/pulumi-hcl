data "simple_lookup" "ds" {
  query = "world"
}

output "lookup_prefix_result" {
  value = data.simple_lookup.ds.prefix_result
}
