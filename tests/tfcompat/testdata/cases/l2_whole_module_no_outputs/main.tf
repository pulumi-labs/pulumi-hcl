# Referencing a whole module that declares no outputs yields an empty object.
# OpenTofu produces {} (and keys() is []); pulumi-hcl must match.
module "child" {
  source = "./child"
}

output "whole_empty" {
  value = module.child
}

output "empty_keys" {
  value = keys(module.child)
}
