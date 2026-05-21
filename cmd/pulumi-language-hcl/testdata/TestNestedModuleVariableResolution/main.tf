terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

module "outer" {
  source = "./outer"
  name   = "my-bucket"
}
output "bucketName" {
  value = module.outer.bucketName
}
