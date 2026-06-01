terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

module "someComponent" {
  source = "./myComponent"
  input  = true
}
output "result" {
  value = module.someComponent.output
}
