terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_bucket" "bucket" {
  lifecycle {
    create_before_destroy = true
  }
  name = var.bucketName
}
variable "bucketName" {
  type = string
}
output "name" {
  value = test_bucket.bucket.name
}
