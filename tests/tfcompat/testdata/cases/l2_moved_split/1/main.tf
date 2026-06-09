module "x" { source = "./mod" }
moved {
  from = simple_resource.a
  to   = module.x.simple_resource.a
}
output "r" { value = module.x.r }
