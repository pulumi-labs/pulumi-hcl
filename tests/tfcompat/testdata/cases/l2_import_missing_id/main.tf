resource "pfx_anon" "a" {
  name = "alpha"
}

output "name" {
  value = pfx_anon.a.name
}
