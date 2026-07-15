provider "simple" {
  prefix = "default"
}

provider "simple" {
  alias  = "special"
  prefix = "special"
}

module "outer" {
  source = "./outer"
  tag    = "a"
  providers = {
    simple = simple.special
  }
}

output "result" {
  value = module.outer.prefix_result
}
