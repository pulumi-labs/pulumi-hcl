variable "name" {
  type = string
}

resource "simple_resource" "g" {
  input_one = var.name
  input_two = false
}

output "result" {
  value = simple_resource.g.prefix_result
}

output "echo" {
  value = "module says ${var.name}"
}
