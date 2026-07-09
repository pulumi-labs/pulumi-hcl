resource "simple_resource" "upstream" {
  input_one = "a"
  input_two = true
}

data "simple_lookup" "lookup" {
  query = simple_resource.upstream.result
}

output "looked_up" {
  value = data.simple_lookup.lookup.prefix_result
}
