terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "res" {
  boolean      = var.boolean
  float        = var.float
  integer      = var.integer
  string       = var.string
  number_array = var.numberArray
  boolean_map  = var.booleanMap
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
variable "numberArray" {
  type = list(number)
}
variable "booleanMap" {
  type = map(bool)
}
