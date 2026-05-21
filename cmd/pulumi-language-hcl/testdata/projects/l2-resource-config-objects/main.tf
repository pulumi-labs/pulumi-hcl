terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "plain" {
  boolean      = true
  float        = 3.5
  integer      = 3
  string       = "plain"
  number_array = var.plainNumberArray
  boolean_map  = var.plainBooleanMap
}
resource "primitive_resource" "secret" {
  boolean      = true
  float        = 3.5
  integer      = 3
  string       = "secret"
  number_array = var.secretNumberArray
  boolean_map  = var.secretBooleanMap
}
variable "plainNumberArray" {
  type = list(number)
}
variable "plainBooleanMap" {
  type = map(bool)
}
variable "secretNumberArray" {
  type = list(number)
}
variable "secretBooleanMap" {
  type = map(bool)
}
