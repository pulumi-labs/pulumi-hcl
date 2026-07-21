module "b" { source = "./mod" }

resource "simple_resource" "r" { input_one = "x" }

# The module call is renamed at the same time as one of its resources moves out
# to the root, so the object created as module.a.simple_resource.r must move to
# simple_resource.r in one apply, with no create or delete.
moved {
  from = module.a
  to   = module.b
}

moved {
  from = module.b.simple_resource.r
  to   = simple_resource.r
}

output "kept" { value = module.b.kept }
output "r" { value = simple_resource.r.result }
