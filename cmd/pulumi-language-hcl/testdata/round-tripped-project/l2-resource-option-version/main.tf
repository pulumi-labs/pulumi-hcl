terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "26.0.0"
    }
  }
}

resource "simple_resource" "withV2" {
  pulumi {
    version = "2.0.0"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "withV26" {
  pulumi {
    version = "26.0.0"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
resource "simple_resource" "withDefault" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
