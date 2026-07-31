resource "blocky_thing" "this" {
  name  = "a"
  alias = ["first"]

  lifecycle {
    ignore_changes = [alias[0]]
  }
}

output "alias" {
  value = blocky_thing.this.alias
}
