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

# Same aliased pass into the middle module, but the innermost module inherits
# the middle module's default `simple` implicitly (no `providers` block on the
# inner call).
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
