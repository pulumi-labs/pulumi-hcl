# `rule` (MaxItems=1) is an optional nested block, left out here. The dynamic
# bridge's wire schema drops the empty nested list, so pulumi-hcl reads
# `policy[0].rule` as null and `length(...)` on it fails the deploy. (OpenTofu
# materializes the absent block as `[]`, length 0 — see the skipped tfcompat
# case of the same name.)
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
