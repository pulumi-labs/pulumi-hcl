# A module-internal reference routed through a local, in a for_each module.
# Verified against real tofu: unlike a direct resource reference (which stays
# per-instance, see l2_module_for_each_instance_flow), a reference THROUGH a
# module local widens across module instances — both `b`s wait for the delayed
# m["y"].a. b-y's own create is also delayed so the two otherwise-concurrent
# `b`s record in a total order: [a-x, a-y, b-x, b-y]. This guards against
# over-narrowing module locals: a runtime that scopes the local per module
# instance records b-x ahead of the delayed a-y — a deterministic order flip.
module "m" {
  source   = "./child"
  for_each = toset(["x", "y"])
  key      = each.key
}

output "results" {
  value = join(",", [for k in sort(keys(module.m)) : module.m[k].result])
}
