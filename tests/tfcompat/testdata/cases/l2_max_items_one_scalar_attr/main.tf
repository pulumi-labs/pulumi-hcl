resource "blocky_thing" "this" {
  name  = "a"
  alias = ["primary"]
}

output "alias" {
  value = blocky_thing.this.alias
}

output "first" {
  value = blocky_thing.this.alias[0]
}
