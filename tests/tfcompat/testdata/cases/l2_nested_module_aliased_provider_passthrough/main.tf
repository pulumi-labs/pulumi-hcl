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

# Like `outer`, but the nested call inherits the passed-in default implicitly.
module "outer_implicit" {
  source = "./outer_implicit"
  tag    = "b"
  providers = {
    simple = simple.special
  }
}

output "result" {
  value = module.outer.prefix_result
}

output "result_implicit" {
  value = module.outer_implicit.prefix_result
}
