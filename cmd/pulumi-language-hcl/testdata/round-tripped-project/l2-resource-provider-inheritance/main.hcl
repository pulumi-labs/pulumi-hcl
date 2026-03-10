terraform {
  required_providers {
    primitive = {
      source  = "pulumi/primitive"
      version = "7.0.0"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "pulumi_providers_simple" "provider" {
}
resource "simple_resource" "parent1" {
  provider = pulumi_providers_simple.provider
  value    = true
}
resource "simple_resource" "child1" {
  parent = simple_resource.parent1
  value  = true
}
resource "primitive_resource" "parent2" {
  boolean      = false
  float        = 0
  integer      = 0
  string       = ""
  number_array = []
  boolean_map  = {}
}
resource "simple_resource" "child2" {
  parent = primitive_resource.parent2
  value  = true
}
resource "primitive_resource" "child3" {
  parent       = simple_resource.parent1
  boolean      = false
  float        = 0
  integer      = 0
  string       = ""
  number_array = []
  boolean_map  = {}
}
