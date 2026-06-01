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

output "echoed_name" {
  value = var.name
}

output "paths" {
  value = {
    module = path.module
    root   = path.root
    cwd    = path.cwd
  }
}
