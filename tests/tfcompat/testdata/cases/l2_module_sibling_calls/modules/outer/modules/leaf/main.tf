variable "name" {
  type = string
}

resource "simple_resource" "r" {
  input_one = var.name
  input_two = false
}

output "result" {
  value = simple_resource.r.prefix_result
}
