resource "simple_resource" "r" {
  input_one = "hello"
  input_two = true
}
locals {
  # Unknown predicate at preview -> a wholly-unknown object({a=string}) that
  # lacks `c`; the module must retype it to the declared object type.
  uo = simple_resource.r.result == "hello-true" ? { a = "one" } : { a = "two" }
}
module "m" {
  source = "./modules/m"
  cfg    = local.uo
}
output "c" { value = module.m.c }
