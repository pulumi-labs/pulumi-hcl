resource "simple_resource" "c_new" {
  count     = 2
  input_one = "c${count.index}"
}

resource "simple_resource" "e_new" {
  for_each  = toset(["x", "y"])
  input_one = each.key
}

module "m" {
  source = "./mod"
}

moved {
  from = simple_resource.c_old
  to   = simple_resource.c_new
}

moved {
  from = simple_resource.e_old
  to   = simple_resource.e_new
}

output "c" { value = [for r in simple_resource.c_new : r.result] }
output "e" { value = { for k, r in simple_resource.e_new : k => r.result } }
output "m" { value = module.m.r }
