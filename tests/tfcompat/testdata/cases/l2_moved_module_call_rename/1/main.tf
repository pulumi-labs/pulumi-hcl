module "b" { source = "./mod" }
moved {
  from = module.a
  to   = module.b
}
output "r" { value = module.b.r }
