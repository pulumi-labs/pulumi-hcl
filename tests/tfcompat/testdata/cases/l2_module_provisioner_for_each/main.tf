# Regression: a module carrying a resource-level provisioner, instantiated more
# than once via for_each. Each instance must register a distinct provisioner
# hook. A hook name derived from the in-module resource address alone (without
# the module instance key) collides across the two instances.
module "runners" {
  source   = "./runner"
  for_each = toset(["a", "b"])
  name     = each.key
}

output "results" {
  value = { for k, m in module.runners : k => m.result }
}
