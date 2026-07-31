resource "blocky_thing" "this" {
  name  = "a"
  alias = ["second"]

  lifecycle {
    ignore_changes = [alias[0]]
  }
}

output "alias" {
  value = blocky_thing.this.alias
}
