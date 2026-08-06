# A module call whose input reaches a resource only through a root local. The
# data source inside the module reads that input, so its read defers until the
# resource is applied.
resource "simple_resource" "res" {
  input_one = "a"
}

locals {
  val = simple_resource.res.result
}

module "child" {
  source = "./modules/child"
  query  = local.val
}

output "out" {
  value = module.child.out
}
