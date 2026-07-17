# The count sibling of l2_module_for_each_instance_flow: each module
# instance's `b` depends only on its own instance's `a`. OpenTofu expands
# modules per count instance, so m[0].b creates as soon as m[0].a is done,
# ahead of the delayed m[1].a: [a-0, b-0, a-1, b-1]. A runtime that widens
# module-internal edges across module instances makes both `b`s wait for the
# delayed a-1 — a deterministic order flip.
module "m" {
  source = "./child"
  count  = 2
  idx    = count.index
}

output "results" {
  value = join(",", [for inst in module.m : inst.result])
}
