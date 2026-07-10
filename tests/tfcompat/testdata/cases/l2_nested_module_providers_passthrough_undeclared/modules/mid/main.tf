module "child" {
  source = "./modules/child"
  providers = {
    simple = simple
  }
}

output "prefix_result" {
  value = module.child.prefix_result
}
