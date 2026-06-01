resource "blocky_thing" "t" {
  name = "nested"

  policy {
    effect = "allow"
    rule {
      action   = "read"
      resource = "*"
    }
  }
}

output "summary" {
  value = blocky_thing.t.summary
}
