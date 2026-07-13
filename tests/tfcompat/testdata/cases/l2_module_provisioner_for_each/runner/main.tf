variable "name" {
  type = string
}

resource "simple_resource" "r" {
  input_one = var.name
  input_two = false

  provisioner "local-exec" {
    command = "true"
  }
}

output "result" {
  value = simple_resource.r.result
}
