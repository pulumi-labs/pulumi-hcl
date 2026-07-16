# `b` depends_on a single for_each instance `a["x"]`. OpenTofu honors the
# instance address: `b` waits only for `a["x"]`, not for the delayed `a["y"]`,
# so the recorded create order is [a["x"], b, a["y"]].
#
# The `a["y"]` create is delayed. With the correct narrow edge, `b` (which does
# not depend on `a["y"]`) records ahead of the delayed `a["y"]`. A runtime that
# widens the edge to the whole resource `a` makes `b` wait for `a["y"]`, so
# `a["y"]` records ahead of `b` — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

resource "order_resource" "b" {
  depends_on = [order_resource.a["x"]]
  name       = "b"
}

output "b_result" {
  value = order_resource.b.result
}
