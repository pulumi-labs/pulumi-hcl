terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

module "comp" {
  source = "./keywordComponent"
  input  = true
}
output "result" {
  value = module.comp.result
}
