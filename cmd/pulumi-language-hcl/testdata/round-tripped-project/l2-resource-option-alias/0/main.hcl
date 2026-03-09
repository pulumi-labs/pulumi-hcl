terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "parent" {
  value = true
}
resource "simple_resource" "aliasURN" {
  value = true
}
resource "simple_resource" "aliasName" {
  value = true
}
resource "simple_resource" "aliasNoParent" {
  value = true
}
resource "simple_resource" "aliasParent" {
  parent = simple_resource.aliasURN
  value  = true
}
