resource "blocky_thing" "t" {
  name = "omit"

  # `rule` (MaxItems=1) is an optional nested block, left out here.
  policy {
    effect = "deny"
  }
}

output "rule" {
  value = blocky_thing.t.policy[0].rule
}

output "rule_len" {
  value = length(blocky_thing.t.policy[0].rule)
}
