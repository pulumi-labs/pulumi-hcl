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
    prevent_destroy       = true
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "unprotected" {
  lifecycle {
    prevent_destroy       = false
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "defaulted" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
