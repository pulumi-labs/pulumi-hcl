# The destroy sibling of l2_depends_on_for_each_instance: `b` depends_on only
# `a["x"]`, so on destroy only `a["x"]` must wait for `b` — `a["y"]` deletes
# immediately. Creates: [a["x"], b, a["y"]] (a["y"]'s create delayed).
# Destroys: [a["y"], b, a["x"]] (b's delete delayed). A runtime that records
# the dependency against the whole resource — or drops it entirely because the
# target is expanded — flips the recorded order deterministically.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

resource "order_resource" "b" {
  depends_on   = [order_resource.a["x"]]
  name         = "b"
  delay_delete = true
}

output "b_result" {
  value = order_resource.b.result
}
