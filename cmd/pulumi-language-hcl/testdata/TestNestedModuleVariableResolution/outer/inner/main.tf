terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

resource "test_bucket" "bucket" {
  name ="inner(${var.name})"
}
variable "name" {
  type = string
}
output "bucketName" {
  value = test_bucket.bucket.name
}
