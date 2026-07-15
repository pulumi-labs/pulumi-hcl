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
  boolean      = var.boolean
  float        = var.float
  integer      = var.integer
  string       = var.string
  number_array = [-1, 0, 1]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
variable "boolean" {
  type = bool
}
variable "float" {
  type = number
}
variable "integer" {
  type = number
}
variable "string" {
  type = string
}
