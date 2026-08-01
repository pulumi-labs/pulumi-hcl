resource "blocky_thing" "this" {
  name  = "a"
  alias = ["primary"]
}

resource "blocky_thing" "nulled" {
  name  = "nulled"
  alias = null
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

output "null_alias" {
  value = jsonencode(blocky_thing.nulled.alias)
}

output "null_is_null" {
  value = blocky_thing.nulled.alias == null
}

output "unset_alias" {
  value = jsonencode(blocky_thing.unset.alias)
}
