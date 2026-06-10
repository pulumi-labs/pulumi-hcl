resource "pfx_thing" "t" {
  name = "alpha"
}

output "name" {
  value = pfx_thing.t.name
}

output "id" {
  value = pfx_thing.t.id
}
