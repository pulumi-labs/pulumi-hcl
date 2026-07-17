# Each module instance's `b` depends only on its own instance's `a`. OpenTofu
# expands modules per instance, so m["x"].b creates as soon as m["x"].a is
# done, ahead of the delayed m["y"].a: [a-x, b-x, a-y, b-y]. A runtime that
# widens module-internal edges across module instances makes both `b`s wait
# for the delayed a-y — a deterministic order flip.
module "m" {
  source   = "./child"
  for_each = toset(["x", "y"])
  key      = each.key
}

output "results" {
  value = join(",", [for k in sort(keys(module.m)) : module.m[k].result])
}
