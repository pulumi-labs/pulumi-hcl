# `b` references a single module instance's output (`module.m["x"].result`).
# Verified against real tofu: unlike an instance-keyed *resource* reference,
# this does NOT narrow — module output references resolve at the module-call
# level, so `b` waits for the delayed m["y"].a as well and the recorded
# create order is [a-x, a-y, b]. This guards against over-narrowing: a
# runtime that lets `b` create after only m["x"] records b ahead of the
# delayed a-y — a deterministic order flip.
module "m" {
  source   = "./child"
  for_each = toset(["x", "y"])
  key      = each.key
}

resource "order_resource" "b" {
  name = "b-${module.m["x"].result}"
}

output "b_result" {
  value = order_resource.b.result
}
