terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "retainOnDelete" {
  retain_on_delete = true
  value            = true
}
resource "simple_resource" "notRetainOnDelete" {
  retain_on_delete = false
  value            = true
}
resource "simple_resource" "defaulted" {
  value = true
}
