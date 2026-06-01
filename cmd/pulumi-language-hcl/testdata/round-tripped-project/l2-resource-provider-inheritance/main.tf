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

provider "simple" {
  alias = "provider"
}
resource "simple_resource" "parent1" {
  provider = simple.provider
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
// This should inherit the explicit provider from parent1
resource "simple_resource" "child1" {
  pulumi {
    parent = simple_resource.parent1
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "primitive_resource" "parent2" {
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 0
  integer      = 0
  string       = ""
  number_array = []
  boolean_map  = {}
}
// This _should not_ inherit the provider from parent2 as it is a default provider.
resource "simple_resource" "child2" {
  pulumi {
    parent = primitive_resource.parent2
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
// This _should not_ inherit the provider from parent1 as its from the wrong package.
resource "primitive_resource" "child3" {
  pulumi {
    parent = simple_resource.parent1
  }
  lifecycle {
    create_before_destroy = true
  }
  boolean      = false
  float        = 0
  integer      = 0
  string       = ""
  number_array = []
  boolean_map  = {}
}
