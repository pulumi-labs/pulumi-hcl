# `b` depends on only a["x"], so it creates before the delayed a["y"]. Destroy
# dependencies are nevertheless resource-wide in OpenTofu: removing the whole
# program makes both a instances wait for b's delayed delete. Delaying a["x"]'s
# delete totals the sibling order.
#
# Creates: [a["x"], b, a["y"]]
# Deletes: [b, a["y"], a["x"]]
#
# A runtime that persists dependency URNs only for siblings registered before
# b omits a["y"], allowing its undelayed delete to record before b on the next
# update, when the old program is not rerun to reconstruct dependencies.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
  delay_delete = each.key == "x"
}

resource "order_resource" "b" {
  depends_on   = [order_resource.a["x"]]
  name         = "b"
  delay_delete = true
}

output "b_result" {
  value = order_resource.b.result
}
