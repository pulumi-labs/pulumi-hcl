provider "simple" {
  prefix = "hello"
}

resource "simple_resource" "a" {
  input_one = "world"
  input_two = true
}

output "prefix_result" {
  value = simple_resource.a.prefix_result
}
