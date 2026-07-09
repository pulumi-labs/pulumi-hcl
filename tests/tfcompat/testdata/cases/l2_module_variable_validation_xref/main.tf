module "child" {
  source = "./child"
}

output "name" {
  value = module.child.name
}
