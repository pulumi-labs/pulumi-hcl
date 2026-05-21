terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

variable "name" {
  type = string
}
module "inner" {
  source = "./inner"
  name   ="outer(${var.name})"
}
output "bucketName" {
  value = module.inner.bucketName
}
