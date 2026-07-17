# Like l2_module_for_each_instance_flow, but the module-internal reference
# from `b` to `a` is routed through a local. OpenTofu evaluates locals per
# module instance, so m["x"].b still creates as soon as m["x"].a is done,
# ahead of the delayed m["y"].a: [a-x, b-x, a-y, b-y]. A runtime that
# evaluates the local once across all module instances makes both `b`s wait
# for the delayed a-y — a deterministic order flip.
module "m" {
  source   = "./child"
  for_each = toset(["x", "y"])
  key      = each.key
}

output "results" {
  value = join(",", [for k in sort(keys(module.m)) : module.m[k].result])
}
