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

output "resource_prefix_result" {
  value = module.child.resource_prefix_result
}

output "data_prefix_result" {
  value = module.child.data_prefix_result
}
