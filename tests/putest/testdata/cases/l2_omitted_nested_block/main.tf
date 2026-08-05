# `rule` (MaxItems=1) is an optional nested block, left out here. It reads as
# an empty list of blocks (`[]`, length 0), matching OpenTofu. The
# terraform-provider plugin path reads it as null instead — the divergence the
# tfcompat case of the same name is skipped for.
resource "blocky_thing" "t" {
  name = "omit"

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
