# Destroy ordering for a body reference to a single for_each instance.
# Verified against real tofu: destroy dependencies are resource-wide — even
# though `b` references only `a["x"]`, BOTH instances' deletes wait for `b`'s
# delayed delete.
#
# Creates: a["x"]'s create is delayed, so `b` (which waits for it) registers
# after both instances exist: [a["y"], a["x"], b]. Destroys: both `a`s wait
# for the delayed `b`; a["x"]'s delete is also delayed so the two otherwise
# concurrent deletes record in a total order: [b, a["y"], a["x"]]. A runtime
# that records no dependency on the expanded target lets the undelayed
# a["y"] delete record ahead of `b` — a deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "x"
  delay_delete = each.key == "x"
}

resource "order_resource" "b" {
  name         = "b-${order_resource.a["x"].result}"
  delay_delete = true
}

output "b_result" {
  value = order_resource.b.result
}
