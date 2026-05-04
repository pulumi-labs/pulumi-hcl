pulumi {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

data "primitive_invoke" "invoke_0" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
data "primitive_invoke" "invoke_1" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
data "primitive_invoke" "invoke_2" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
data "primitive_invoke" "invoke_3" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
data "primitive_invoke" "invoke_4" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
data "primitive_invoke" "invoke_5" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainBool
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}

resource "primitive_resource" "plainValues" {
  boolean      = var.plainString
  float        = var.plainInteger
  integer      = var.plainNumericString
  string       = var.plainNumber
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
resource "primitive_resource" "secretValues" {
  boolean      = var.secretString
  float        = var.secretInteger
  integer      = var.secretNumericString
  string       = var.secretNumber
  number_array = [var.plainInteger, var.plainNumericString, var.plainNumber]
  boolean_map = {
    "fromBool"   = var.plainBool
    "fromString" = var.plainString
  }
}
resource "primitive_resource" "invokeValues" {
  boolean      = data.primitive_invoke.invoke_0.boolean
  float        = data.primitive_invoke.invoke_1.float
  integer      = data.primitive_invoke.invoke_2.integer
  string       = data.primitive_invoke.invoke_3.string
  number_array = data.primitive_invoke.invoke_4.number_array
  boolean_map  = data.primitive_invoke.invoke_5.boolean_map
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
variable "plainNumericString" {
  type = string
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
variable "secretNumericString" {
  type = string
}
