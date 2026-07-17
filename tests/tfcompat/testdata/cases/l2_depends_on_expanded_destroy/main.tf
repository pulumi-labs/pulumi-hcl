# `b` depends_on the whole of an expanded (for_each) resource `a`, with no
# body reference between them. The depends_on edge must govern destroy
# ordering: `a["x"]`'s delete waits for `b`'s delayed delete, so the recorded
# sequence is [create a["x"], create b, delete b, delete a["x"]]. A runtime
# that fails to resolve depends_on against expanded instance state records no
# dependency, letting a["x"]'s undelayed delete record ahead of b — a
# deterministic order flip.
resource "order_resource" "a" {
  for_each     = toset(["x"])
  name         = each.key
  delay_create = true
}

resource "order_resource" "b" {
  depends_on   = [order_resource.a]
  name         = "b"
  delay_delete = true
}

output "b_result" {
  value = order_resource.b.result
}
