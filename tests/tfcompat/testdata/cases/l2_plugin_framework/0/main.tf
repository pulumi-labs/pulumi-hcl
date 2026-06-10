resource "pfx_thing" "t" {
  name = "alpha"
}

data "pfx_lookup" "l" {}

output "name" {
  value = pfx_thing.t.name
}

output "id" {
  value = pfx_thing.t.id
}

output "lookup" {
  value = data.pfx_lookup.l.value
}
