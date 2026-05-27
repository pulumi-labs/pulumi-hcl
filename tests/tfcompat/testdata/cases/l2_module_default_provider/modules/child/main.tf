provider "simple" {
  prefix = "module-prefix"
}

resource "simple_resource" "r" {
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
