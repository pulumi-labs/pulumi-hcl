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
  parent = simple_resource.parent1
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
  retain_on_delete = true
  lifecycle {
    prevent_destroy       = true
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "child2" {
  parent = simple_resource.parent2
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "child3" {
  parent           = simple_resource.parent2
  retain_on_delete = false
  lifecycle {
    prevent_destroy       = false
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
