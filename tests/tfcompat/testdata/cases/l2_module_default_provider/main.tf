module "child" {
  source = "./modules/child"
}

output "module_prefix_result" {
  value = module.child.prefix_result
}
