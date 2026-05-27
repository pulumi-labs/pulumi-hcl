module "child" {
  source = "./modules/child"
}

output "lookup_prefix_result" {
  value = module.child.lookup_prefix_result
}
