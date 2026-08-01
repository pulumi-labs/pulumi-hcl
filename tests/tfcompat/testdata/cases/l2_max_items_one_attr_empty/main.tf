resource "blocky_thing" "empty" {
  name  = "empty"
  alias = []
}

output "empty_alias" {
  value = jsonencode(blocky_thing.empty.alias)
}

output "empty_is_null" {
  value = blocky_thing.empty.alias == null
}
