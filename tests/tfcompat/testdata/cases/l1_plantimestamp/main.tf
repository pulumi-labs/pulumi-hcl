# `plantimestamp` returns the plan-time RFC3339 timestamp. Its value is
# nondeterministic, so the outputs derive deterministic facts from it: the
# RFC3339 second-precision string is always 20 characters, and the year is a
# number greater than 2000.
locals {
  now = plantimestamp()
}

output "len" {
  value = length(local.now)
}

output "year_gt_2000" {
  value = tonumber(formatdate("YYYY", local.now)) > 2000
}
