terraform {
  required_providers {
    config = {
      source  = "pulumi/config"
      version = "9.0.0"
    }
    multi-argument-invoke = {
      source  = "pulumi/multi-argument-invoke"
      version = "44.0.0"
    }
  }
}

provider "config" {
  alias = "prov"
  name  = "my config"
}
module "myComponent" {
  source = "./invokeComponent"
  providers = {
    config = config.prov
  }
}
