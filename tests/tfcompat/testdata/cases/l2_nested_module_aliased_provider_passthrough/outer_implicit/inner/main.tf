terraform {
  required_providers {
    simple = {
      source = "hashicorp/simple"
    }
  }
}

variable "tag" {
  type = string
}

resource "simple_resource" "r" {
  input_one = var.tag
}

output "prefix_result" {
  value = simple_resource.r.prefix_result
}
