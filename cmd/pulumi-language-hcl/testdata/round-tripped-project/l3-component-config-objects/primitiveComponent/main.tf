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
  boolean      = true
  float        = 3.5
  integer      = 3
  string       = "plain"
  number_array = var.numberArray
  boolean_map  = var.booleanMap
}
variable "numberArray" {
  type = list(number)
}
variable "booleanMap" {
  type = map(bool)
}
