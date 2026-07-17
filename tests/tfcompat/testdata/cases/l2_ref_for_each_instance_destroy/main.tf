# The destroy sibling of l2_ref_for_each_instance: `b` references only
# `a["x"]`, so on destroy only `a["x"]` must wait for `b` — `a["y"]` deletes
# immediately. Creates: [a["x"], b, a["y"]] (a["y"]'s create delayed).
# Destroys: [a["y"], b, a["x"]] (b's delete delayed, so the undelayed a["y"]
# records first and a["x"] must record after b). A runtime that records a
# dependency on the whole resource makes a["y"]'s delete wait for b — its
# delete then records after the delayed b, a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

resource "order_resource" "b" {
  name         = "b-${order_resource.a["x"].result}"
  delay_delete = true
}

output "b_result" {
  value = order_resource.b.result
}
