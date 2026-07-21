module "b" { source = "./mod" }

# The module call is renamed at the same time as the resource inside it, so the
# object created as module.a.simple_resource.old must move to
# module.b.simple_resource.new in one apply, with no create or delete.
moved {
  from = module.a
  to   = module.b
}

output "r" { value = module.b.r }
