terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "simple_resource" "target" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
resource "simple_resource" "other" {
  lifecycle {
    create_before_destroy = true
  }
  value = true
}
