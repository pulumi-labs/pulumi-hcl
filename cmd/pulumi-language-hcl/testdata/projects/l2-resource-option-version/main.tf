terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "26.0.0"
    }
  }
}

resource "simple_resource" "withV2" {
  lifecycle {
    create_before_destroy = true
  }
  version = "2.0.0"
  value   = true
}
resource "simple_resource" "withV26" {
  lifecycle {
    create_before_destroy = true
  }
  version = "26.0.0"
  value   = false
}
resource "simple_resource" "withDefault" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
