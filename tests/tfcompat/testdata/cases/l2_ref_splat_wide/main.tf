# `b` references `a` through a splat (`a[*].result`), which addresses every
# instance: `b` must wait for the delayed `a[1]` as well, so the recorded
# create order is [a[0], a[1], b]. This guards against over-narrowing: a
# runtime that treats the splat's index step as a single-instance address lets
# `b` record ahead of the delayed a[1] — a deterministic order flip.
resource "order_resource" "a" {
  count        = 2
  name         = "a-${count.index}"
  delay_create = count.index == 1
}

resource "order_resource" "b" {
  name = "b-${join(",", order_resource.a[*].result)}"
}

output "b_result" {
  value = order_resource.b.result
}
