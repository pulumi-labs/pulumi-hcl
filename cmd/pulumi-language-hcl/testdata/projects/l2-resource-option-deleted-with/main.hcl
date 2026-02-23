terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "target" {
  value = true
}
resource "simple_resource" "deletedWith" {
  deleted_with = simple_resource.target
  value        = true
}
resource "simple_resource" "notDeletedWith" {
  value = true
}
