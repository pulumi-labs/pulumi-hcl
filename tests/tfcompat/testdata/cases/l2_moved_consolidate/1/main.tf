module "a" { source = "./mod" }

resource "simple_resource" "r" { input_one = "x" }

# Moving a resource out of a module back to the root. The module remains, so its
# component is still registered; the resource is aliased to the name it had under
# that component.
moved {
  from = module.a.simple_resource.r
  to   = simple_resource.r
}

output "kept" { value = module.a.kept }
output "r" { value = simple_resource.r.result }
