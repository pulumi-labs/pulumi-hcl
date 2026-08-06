variable "query" {
  type = string
}

data "simple_lookup" "lookup" {
  query = var.query
}

output "out" {
  value = data.simple_lookup.lookup.prefix_result
}
