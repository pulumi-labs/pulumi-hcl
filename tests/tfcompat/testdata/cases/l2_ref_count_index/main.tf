# `b` references a single count instance `a[0]` in its body. OpenTofu resolves
# a literal-indexed reference to the exact instance: `b` waits only for `a[0]`,
# not for the delayed `a[1]`, so the recorded create order is [a[0], b, a[1]].
resource "order_resource" "a" {
  count        = 2
  name         = "a-${count.index}"
  delay_create = count.index == 1
}

resource "order_resource" "b" {
  name = "b-${order_resource.a[0].result}"
}

output "b_result" {
  value = order_resource.b.result
}
