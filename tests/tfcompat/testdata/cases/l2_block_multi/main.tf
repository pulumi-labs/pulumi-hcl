resource "blocky_thing" "t" {
  name = "multi"

  tag {
    key   = "env"
    value = "prod"
  }
  tag {
    key   = "team"
    value = "platform"
  }
}

output "summary" {
  value = blocky_thing.t.summary
}
