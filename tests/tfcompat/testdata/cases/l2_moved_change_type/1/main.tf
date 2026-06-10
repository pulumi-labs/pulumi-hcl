resource "pfx_widget" "t" {
  name = "alpha"
}

moved {
  from = pfx_thing.t
  to   = pfx_widget.t
}

output "name" {
  value = pfx_widget.t.name
}

output "id" {
  value = pfx_widget.t.id
}
