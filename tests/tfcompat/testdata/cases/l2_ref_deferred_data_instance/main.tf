# `b` references a single instance of a for_each data source whose reads are
# deferred to apply (they depend on resource `a`). In OpenTofu's apply graph
# each read is its own node, so `b` waits only for the `d["x"]` read, not for
# the delayed `d["y"]` read: [create a, read d["x"], create b, read d["y"]].
# A runtime that widens the reference to the whole data source makes `b` wait
# for the delayed d["y"] — a deterministic order flip.
resource "order_resource" "a" {
  name = "a"
}

data "order_data" "d" {
  for_each   = toset(["x", "y"])
  name       = "${order_resource.a.result}-${each.key}"
  delay_read = each.key == "y"
}

resource "order_resource" "b" {
  name = "b-${data.order_data.d["x"].result}"
}

output "b_result" {
  value = order_resource.b.result
}
