module "b" { source = "./mod" }

# Moving a resource from one non-root module to another. The `from` lives in a
# different module than the resource being registered, so the alias would need
# that module's prior component URN — not yet supported.
moved {
  from = module.a.simple_resource.r
  to   = module.b.simple_resource.r
}

output "r" { value = module.b.r }
