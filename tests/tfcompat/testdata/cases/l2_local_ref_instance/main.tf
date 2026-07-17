# `b` references `a["x"]` only through a local. OpenTofu resolves references
# through locals transitively, keeping the instance address narrow: `b` waits
# only for `a["x"]`, not for the delayed `a["y"]`, so the recorded create
# order is [a["x"], b, a["y"]]. A runtime that widens the reference at the
# local makes `b` wait for `a["y"]` too — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

locals {
  picked = order_resource.a["x"].result
}

resource "order_resource" "b" {
  name = "b-${local.picked}"
}

output "b_result" {
  value = order_resource.b.result
}
