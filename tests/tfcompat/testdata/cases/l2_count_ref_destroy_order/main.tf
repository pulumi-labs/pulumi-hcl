resource "order_resource" "a" {
  name         = "a"
  delay_create = true
}

# `b` references `a` only through its `count` meta-argument. TF records this as
# a dependency, so the recorded sequence is [create a, create b, delete b,
# delete a]. The reference never appears in `b`'s body, so a runtime that only
# derives dependencies from input values will miss the edge; the op that must
# complete first in each phase is delayed, so a missing edge flips the
# recorded order deterministically.
resource "order_resource" "b" {
  count        = order_resource.a.name == "a" ? 1 : 0
  name         = "b"
  delay_delete = true
}

output "a_result" {
  value = order_resource.a.result
}
