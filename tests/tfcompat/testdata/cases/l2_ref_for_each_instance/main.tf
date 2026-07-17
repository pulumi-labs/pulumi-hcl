# `b` references a single for_each instance `a["x"]` in its body. OpenTofu
# resolves a literal-indexed reference to the exact instance: `b` waits only
# for `a["x"]`, not for the delayed `a["y"]`, so the recorded create order is
# [a["x"], b, a["y"]]. A runtime that widens the reference to the whole
# resource `a` makes `b` wait for `a["y"]` too — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

resource "order_resource" "b" {
  name = "b-${order_resource.a["x"].result}"
}

output "b_result" {
  value = order_resource.b.result
}
