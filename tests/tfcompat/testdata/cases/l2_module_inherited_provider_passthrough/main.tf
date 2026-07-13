provider "simple" {
  prefix = "root-prefix"
}

# `child` gets no `providers` block on this call, so it inherits the root
# default `simple`. `child` then passes that inherited default down to its own
# child via a `providers` block — the case the Materialize wrapper hits.
module "child" {
  source = "./modules/child"
}

output "result" {
  value = module.child.result
}
