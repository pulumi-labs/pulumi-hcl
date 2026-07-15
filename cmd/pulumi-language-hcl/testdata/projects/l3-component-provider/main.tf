terraform {
  required_providers {
    config = {
      source  = "pulumi/config"
      version = "9.0.0"
    }
  }
}

module "myComponent" {
  source = "./providerComponent"
  text   = "hello"
}
output "result" {
  value = module.myComponent.result
}
