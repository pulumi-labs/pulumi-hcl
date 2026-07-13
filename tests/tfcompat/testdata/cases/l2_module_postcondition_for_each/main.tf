# Regression: a module carrying a resource-level postcondition, instantiated
# more than once via for_each. Each instance must register a distinct
# postcondition hook; a name derived from the in-module resource address alone
# collides.
module "checks" {
  source   = "./runner"
  for_each = toset(["a", "b"])
  name     = each.key
}

output "results" {
  value = { for k, m in module.checks : k => m.result }
}
