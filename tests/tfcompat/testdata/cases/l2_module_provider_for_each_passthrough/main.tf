variable "prefixes" {
  type = map(string)
  default = {
    a = "alpha"
    b = "beta"
  }
}

provider "simple" {
  alias    = "by_key"
  for_each = var.prefixes
  prefix   = each.value
}

module "child" {
  source = "./modules/child"
  providers = {
    simple = simple.by_key["b"]
  }
}

output "module_prefix_result" {
  value = module.child.prefix_result
}
