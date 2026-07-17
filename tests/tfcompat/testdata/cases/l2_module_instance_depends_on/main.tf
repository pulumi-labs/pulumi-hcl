# `b` depends_on a single module instance (`module.m["x"]`). Verified against
# real tofu: the instance key is accepted but the dependency applies to the
# whole module call — `b` waits for the delayed m["y"].a as well, so the
# recorded create order is [a-x, a-y, b]. This guards against over-narrowing:
# a runtime that honors the instance key records b ahead of the delayed a-y —
# a deterministic order flip.
module "m" {
  source   = "./child"
  for_each = toset(["x", "y"])
  key      = each.key
}

resource "order_resource" "b" {
  depends_on = [module.m["x"]]
  name       = "b"
}

output "b_result" {
  value = order_resource.b.result
}
