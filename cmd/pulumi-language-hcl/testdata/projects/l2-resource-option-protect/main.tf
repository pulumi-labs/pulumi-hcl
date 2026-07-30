terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "protected" {
  pulumi {
    protect = true
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "unprotected" {
  pulumi {
    protect = false
  }
  lifecycle {
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
