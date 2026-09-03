terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_item" "inner" {
  pulumi {
    name ="${pulumi.module.name}-inner"
  }
  lifecycle {
    create_before_destroy = true
  }
  value = var.source
}
variable "source" {
  type = string
}
