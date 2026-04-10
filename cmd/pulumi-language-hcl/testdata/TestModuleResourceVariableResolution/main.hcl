terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

module "mod" {
  source     = "./mod"
  bucketName = "my-bucket"
}
output "name" {
  value = module.mod.name
}
