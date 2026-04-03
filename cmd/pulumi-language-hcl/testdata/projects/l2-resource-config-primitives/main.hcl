terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "primitive_resource" "plain" {
  boolean      = var.plainBool
  float        = var.plainNumber
  integer      = var.plainInteger
  string       = var.plainString
  number_array = [-1, 0, 1]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
resource "primitive_resource" "secret" {
  boolean      = var.secretBool
  float        = var.secretNumber
  integer      = var.secretInteger
  string       = var.secretString
  number_array = [-2, 0, 2]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
variable "plainBool" {
  type = bool
}
variable "plainNumber" {
  type = number
}
variable "plainInteger" {
  type = number
}
variable "plainString" {
  type = string
}
variable "secretBool" {
  type = bool
}
variable "secretNumber" {
  type = number
}
variable "secretInteger" {
  type = number
}
variable "secretString" {
  type = string
}
