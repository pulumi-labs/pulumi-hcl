terraform {
  required_providers {
    component = {
      source  = "pulumi/component"
      version = "13.3.7"
    }
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

provider "simple" {
  alias = "explicit"
}
module "withProviders" {
  source = "./local"
  providers = {
    simple = simple.explicit
  }
}
output "result" {
  value = module.withProviders.result
}
