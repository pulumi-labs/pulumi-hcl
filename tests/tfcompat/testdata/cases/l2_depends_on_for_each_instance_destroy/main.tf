# Destroy ordering for an instance-addressed depends_on. Verified against
# real tofu: destroy dependencies are resource-wide — even though `b`
# depends_on only `a["x"]`, BOTH instances' deletes wait for `b`'s delayed
# delete.
#
# Creates: a["x"]'s create is delayed, so `b` (whose depends_on waits for it)
# registers after both instances exist: [a["y"], a["x"], b]. Destroys: both
# `a`s wait for the delayed `b`; a["x"]'s delete is also delayed so the two
# otherwise concurrent deletes record in a total order: [b, a["y"], a["x"]].
# A runtime that drops the dependency because the target is expanded lets the
# undelayed a["y"] delete record ahead of `b` — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "x"
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
