terraform {
  required_providers {
    optionalprimitive = {
      source  = "pulumi/optionalprimitive"
      version = "34.0.0"
    }
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

resource "optionalprimitive_resource" "unsetA" {
}
resource "optionalprimitive_resource" "unsetB" {
  boolean      = optionalprimitive_resource.unsetA.boolean
  float        = optionalprimitive_resource.unsetA.float
  integer      = optionalprimitive_resource.unsetA.integer
  string       = optionalprimitive_resource.unsetA.string
  number_array = optionalprimitive_resource.unsetA.number_array
  boolean_map  = optionalprimitive_resource.unsetA.boolean_map
}
resource "optionalprimitive_resource" "setA" {
  boolean      = true
  float        = 3.14
  integer      = 42
  string       = "hello"
  number_array = [-1, 0, 1]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
resource "optionalprimitive_resource" "setB" {
  boolean      = optionalprimitive_resource.setA.boolean
  float        = optionalprimitive_resource.setA.float
  integer      = optionalprimitive_resource.setA.integer
  string       = optionalprimitive_resource.setA.string
  number_array = optionalprimitive_resource.setA.number_array
  boolean_map  = optionalprimitive_resource.setA.boolean_map
}
resource "primitive_resource" "sourcePrimitive" {
  boolean      = true
  float        = 3.14
  integer      = 42
  string       = "hello"
  number_array = [-1, 0, 1]
  boolean_map = {
    "t" = true
    "f" = false
  }
}
resource "optionalprimitive_resource" "fromPrimitive" {
  boolean      = primitive_resource.sourcePrimitive.boolean
  float        = primitive_resource.sourcePrimitive.float
  integer      = primitive_resource.sourcePrimitive.integer
  string       = primitive_resource.sourcePrimitive.string
  number_array = primitive_resource.sourcePrimitive.number_array
  boolean_map  = primitive_resource.sourcePrimitive.boolean_map
}
output "unsetBoolean" {
  value = optionalprimitive_resource.unsetB.boolean == null ? "null" : "not null"
}
output "unsetFloat" {
  value = optionalprimitive_resource.unsetB.float == null ? "null" : "not null"
}
output "unsetInteger" {
  value = optionalprimitive_resource.unsetB.integer == null ? "null" : "not null"
}
output "unsetString" {
  value = optionalprimitive_resource.unsetB.string == null ? "null" : "not null"
}
output "unsetNumberArray" {
  value = optionalprimitive_resource.unsetB.number_array == null ? "null" : "not null"
}
output "unsetBooleanMap" {
  value = optionalprimitive_resource.unsetB.boolean_map == null ? "null" : "not null"
}
output "setBoolean" {
  value = optionalprimitive_resource.setB.boolean
}
output "setFloat" {
  value = optionalprimitive_resource.setB.float
}
output "setInteger" {
  value = optionalprimitive_resource.setB.integer
}
output "setString" {
  value = optionalprimitive_resource.setB.string
}
output "setNumberArray" {
  value = optionalprimitive_resource.setB.number_array
}
output "setBooleanMap" {
  value = optionalprimitive_resource.setB.boolean_map
}
