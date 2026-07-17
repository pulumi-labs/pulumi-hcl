# A data source whose config is fully known reads during OpenTofu's plan
# phase, and apply starts only after plan completes. `b` references only
# `d["x"]`, yet its create still records after BOTH reads (including the
# delayed `d["y"]`): [read d["x"], read d["y"], create b]. This guards
# against over-narrowing: a runtime that lets `b` create as soon as `d["x"]`
# is read records b ahead of the delayed d["y"] — a deterministic flip.
data "order_data" "d" {
  for_each   = toset(["x", "y"])
  name       = each.key
  delay_read = each.key == "y"
}

resource "order_resource" "b" {
  name = "b-${data.order_data.d["x"].result}"
}

output "b_result" {
  value = order_resource.b.result
}
