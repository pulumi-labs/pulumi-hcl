module "child" {
  source = "./child"
}

output "child_result" {
  value = module.child.result
}
