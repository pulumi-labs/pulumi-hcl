# `pending_lookup` errors unless a `pending_thing` of that name already exists,
# and its own arguments are fully known: only `depends_on` ties it to the
# resource that makes the read succeed.
resource "pending_thing" "thing" {
  name = "widget"
}

data "pending_lookup" "lookup" {
  name = "widget"

  depends_on = [pending_thing.thing]
}

output "looked_up" {
  value = data.pending_lookup.lookup.result
}
