module "child" {
  source = "./child"
  name   = "x"
}

output "name" {
  value = module.child.name
}
