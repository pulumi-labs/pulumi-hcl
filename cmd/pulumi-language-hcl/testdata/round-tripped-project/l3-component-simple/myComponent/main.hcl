terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "res" {
  value = var.input
}
variable "input" {
  type = bool
}
output "output" {
  value = simple_resource.res.value
}
