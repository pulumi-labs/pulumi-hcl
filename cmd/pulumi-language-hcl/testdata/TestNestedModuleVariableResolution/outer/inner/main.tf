terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_bucket" "bucket" {
  pulumi {
    name ="${pulumi.module.name}-bucket"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="inner(${var.name})"
}
variable "name" {
  type = string
}
output "bucketName" {
  value = test_bucket.bucket.name
}
