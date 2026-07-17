# A resource over a set feeding a data source over the same set through a
# dynamic index (`a[each.key]`). The dynamic index means the reference is to
# the whole resource, so every `d` read waits for every `a` instance
# (including the delayed `a["y"]`): [a["x"], a["y"], d["x"], d["y"]], with
# `d["y"]`'s read delayed to keep the two reads totally ordered.
resource "order_resource" "a" {
  for_each     = toset(["x", "y"])
  name         = each.key
  delay_create = each.key == "y"
}

data "order_data" "d" {
  for_each   = toset(["x", "y"])
  name       = order_resource.a[each.key].result
  delay_read = each.key == "y"
}

output "d_results" {
  value = join(",", [for k in sort(keys(data.order_data.d)) : data.order_data.d[k].result])
}
