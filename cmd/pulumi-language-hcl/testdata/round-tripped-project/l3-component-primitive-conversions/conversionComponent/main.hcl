pulumi {
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
  number_array = [2, 42, 6.5]
  boolean_map = {
    "fromBool"   = true
    "fromString" = true
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
output "boolean" {
  value = primitive_resource.res.boolean
}
output "float" {
  value = primitive_resource.res.float
}
output "integer" {
  value = primitive_resource.res.integer
}
output "string" {
  value = primitive_resource.res.string
}
