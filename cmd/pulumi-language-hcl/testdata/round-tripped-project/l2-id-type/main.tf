terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
  }
}

// Test that the ID type is treated the same as a string type, despite being tracked as a distinct type. This 
// includes directly passing it to string fields, but also for bool and numeric values being able to cast to it.
resource "primitive_resource" "source1" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 1
  integer      = 2
  string       = "1234"
  number_array = [3]
  boolean_map = {
    "source" = false
  }
}
resource "primitive_resource" "source2" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 1
  integer      = 2
  string       = "true"
  number_array = [3]
  boolean_map = {
    "source" = false
  }
}
resource "primitive_resource" "sink1" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = local.idMap["source1Token"]
  integer      = local.idMap["source1Token"]
  string       = local.idMap["source1Token"]
  number_array = [local.idMap["source1Token"]]
  boolean_map = {
    "sink" = false
  }
}
resource "primitive_resource" "sink2" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = local.idMap["source2Token"]
  float        = 1
  integer      = 2
  string       = "abc"
  number_array = [3]
  boolean_map = {
    "sink" = local.idMap["source2Token"]
  }
}
locals {
  idMap = {
    "source1Token" = primitive_resource.source1.id
    "source2Token" = primitive_resource.source2.id
  }
}
output "ids" {
  value = local.idMap
}
// test an id value can flow through a string function
output "base64" {
  value = base64encode(primitive_resource.sink2.id)
}
