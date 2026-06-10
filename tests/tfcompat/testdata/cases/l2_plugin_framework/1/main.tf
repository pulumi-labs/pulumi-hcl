resource "pfx_thing" "t" {
  name = "beta"
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
