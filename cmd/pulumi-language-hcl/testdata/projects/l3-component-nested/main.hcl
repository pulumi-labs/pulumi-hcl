terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

module "outerComponent" {
  source = "./outerComponent"
  input  = true
}
output "result" {
  value = module.outerComponent.output
}
