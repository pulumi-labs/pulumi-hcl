resource "simple_resource" "r" {
  input_one = "hello"
  input_two = true
}
locals {
  # Unknown predicate at preview -> a wholly-unknown object({a=string}) that
  # lacks `c`. OpenTofu converts it to the declared object type (adding `c`),
  # so var.cfg.c is valid. If pulumi skips the conversion for the unknown
  # value, var.cfg stays object({a}) and var.cfg.c errors at preview.
  uo = simple_resource.r.result == "hello-true" ? { a = "one" } : { a = "two" }
}
module "m" {
  source = "./modules/m"
  cfg    = local.uo
}
output "c" { value = module.m.c }
