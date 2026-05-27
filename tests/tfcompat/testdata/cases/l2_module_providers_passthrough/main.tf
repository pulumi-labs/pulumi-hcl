provider "simple" {
  alias  = "configured"
  prefix = "from-parent"
}

module "child" {
  source = "./modules/child"
  providers = {
    simple = simple.configured
  }
}

output "module_prefix_result" {
  value = module.child.prefix_result
}
