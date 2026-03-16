terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

variable "value" {
  type = bool
}

resource "simple_resource" "res-child" {
  value = !var.value
}

output "value" {
  value = var.value
}
