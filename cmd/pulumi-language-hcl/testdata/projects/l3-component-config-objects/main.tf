terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
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
module "plain" {
  source      = "./primitiveComponent"
  numberArray = var.plainNumberArray
  booleanMap  = var.plainBooleanMap
}
module "secret" {
  source      = "./primitiveComponent"
  numberArray = var.secretNumberArray
  booleanMap  = var.secretBooleanMap
}
