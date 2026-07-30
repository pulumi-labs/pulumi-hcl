terraform {
  required_providers {
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
resource "simple_resource" "child1" {
  pulumi {
    parent = simple_resource.parent1
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "orphan1" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "parent2" {
  pulumi {
    protect          = true
    retain_on_delete = true
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "child2" {
  pulumi {
    parent = simple_resource.parent2
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "child3" {
  pulumi {
    parent           = simple_resource.parent2
    protect          = false
    retain_on_delete = false
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "orphan2" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
