terraform {
  required_providers {
    test = {
      source  = "pulumi/test"
      version = "1.0.0"
    }
  }
}

module "mod" {
  source = "./mod"
  items  = ["a", "b", "c"]
}
output "result" {
  value = module.mod.result
}
