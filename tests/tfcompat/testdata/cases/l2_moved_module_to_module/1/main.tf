module "b" { source = "./mod" }

# Moving a resource from one non-root module to another. The resource is aliased
# to the name and parent component it had under the prior module.
moved {
  from = module.a.simple_resource.r
  to   = module.b.simple_resource.r
}

output "r" { value = module.b.r }
