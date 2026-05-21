terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "protected" {
  lifecycle {
    prevent_destroy = true
  }
  value = true
}
resource "simple_resource" "unprotected" {
  lifecycle {
    prevent_destroy = false
  }
  value = true
}
resource "simple_resource" "defaulted" {
  value = true
}
