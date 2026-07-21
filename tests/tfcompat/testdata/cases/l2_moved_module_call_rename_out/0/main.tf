module "a" { source = "./mod" }
output "kept" { value = module.a.kept }
