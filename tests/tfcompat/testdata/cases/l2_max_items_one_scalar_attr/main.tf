resource "blocky_thing" "this" {
  name  = "a"
  alias = ["primary"]
}

resource "blocky_thing" "empty" {
  name  = "empty"
  alias = []
}

resource "blocky_thing" "unset" {
  name = "unset"
}

output "alias" {
  value = blocky_thing.this.alias
}

output "first" {
  value = blocky_thing.this.alias[0]
}

output "empty_alias" {
  value = jsonencode(blocky_thing.empty.alias)
}

output "empty_is_null" {
  value = blocky_thing.empty.alias == null
}

output "unset_alias" {
  value = jsonencode(blocky_thing.unset.alias)
}
