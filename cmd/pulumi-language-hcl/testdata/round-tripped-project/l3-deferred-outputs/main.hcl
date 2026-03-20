terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

module "first" {
  source = "./first"
  input  = module.second.untainted
}
module "second" {
  source = "./second"
  input  = module.first.untainted
}
module "another" {
  source = "./first"
  input  = join("", [for _, v in module.many : v.untainted ? "a" : "b"]) == "xyz"
}
module "many" {
  source = "./second"
  count  = 2
  input  = module.another.untainted
}
