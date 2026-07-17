# A data source references a single for_each instance (`a["x"].result`). The
# resource dependency defers the read to the apply phase, where OpenTofu
# resolves the literal-indexed reference to the exact instance: the read waits
# only for `a["x"]`, not for the delayed `a["y"]`, so the recorded order is
# [create a["x"], read d, create a["y"]]. A runtime that widens the reference
# to the whole resource reads after `a["y"]` — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

data "order_data" "d" {
  name = order_resource.a["x"].result
}

output "d_result" {
  value = data.order_data.d.result
}
