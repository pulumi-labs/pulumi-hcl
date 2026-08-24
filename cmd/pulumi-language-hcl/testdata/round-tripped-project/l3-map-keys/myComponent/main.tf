terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "res" {
  pulumi {
    name ="${pulumi.module.name}-res"
  }
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 2.17
  integer      = -12
  string       = "adversarial"
  number_array = [0, 1]
  boolean_map  = var.booleanMap
}
variable "booleanMap" {
  type = map(bool)
}
output "booleanMap" {
  value = primitive_resource.res.boolean_map
}
