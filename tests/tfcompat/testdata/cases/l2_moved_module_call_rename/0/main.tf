module "a" { source = "./mod" }
output "r" { value = module.a.r }
