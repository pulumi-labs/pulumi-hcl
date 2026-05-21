terraform {
  required_providers {
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
resource "simple_resource" "orphan1" {
  value = true
}
resource "simple_resource" "parent2" {
  retain_on_delete = true
  lifecycle {
    prevent_destroy = true
  }
  value = true
}
resource "simple_resource" "child2" {
  parent = simple_resource.parent2
  value  = true
}
resource "simple_resource" "child3" {
  parent           = simple_resource.parent2
  retain_on_delete = false
  lifecycle {
    prevent_destroy = false
  }
  value = true
}
resource "simple_resource" "orphan2" {
  value = true
}
