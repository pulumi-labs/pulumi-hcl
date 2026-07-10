module "child" {
  source = "./modules/child"
  providers = {
    simple = simple
  }
}

output "module_prefix_result" {
  value = module.child.prefix_result
}
