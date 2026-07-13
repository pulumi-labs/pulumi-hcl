# Regression: a module carrying a resource-level precondition, instantiated more
# than once via for_each. Each instance must register a distinct precondition
# hook; a name derived from the in-module resource address alone collides.
module "guards" {
  source   = "./runner"
  for_each = toset(["a", "b"])
  name     = each.key
}

output "results" {
  value = { for k, m in module.guards : k => m.result }
}
