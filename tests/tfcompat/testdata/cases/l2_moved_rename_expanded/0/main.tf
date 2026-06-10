resource "simple_resource" "c_old" {
  count     = 2
  input_one = "c${count.index}"
}

resource "simple_resource" "e_old" {
  for_each  = toset(["x", "y"])
  input_one = each.key
}

module "m" {
  source = "./mod"
}

output "c" { value = [for r in simple_resource.c_old : r.result] }
output "e" { value = { for k, r in simple_resource.e_old : k => r.result } }
output "m" { value = module.m.r }
