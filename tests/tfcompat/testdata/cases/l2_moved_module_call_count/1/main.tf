module "a" {
  source = "./mod"
  count  = 1
}
moved {
  from = module.a
  to   = module.a[0]
}
output "r" { value = module.a[0].r }
