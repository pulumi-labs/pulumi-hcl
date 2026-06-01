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

output "lookup_prefix_result" {
  value = module.child.lookup_prefix_result
}
