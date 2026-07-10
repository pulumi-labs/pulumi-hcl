# The YAML merge key `<<` may reference a sequence of mappings, each of which
# is merged into the enclosing mapping. `result` below merges both `base1` and
# `base2` (with `p` pinning cross-merge precedence), then overrides with its
# own keys. `multi` uses two separate `<<` keys in one mapping, and `scalar.v`
# uses `<<` outside key position, where it is a plain string.
locals {
  doc = <<-YAML
    base1: &b1
      a: 1
      p: from1
    base2: &b2
      c: 3
      p: from2
    result:
      <<: [*b1, *b2]
      b: 20
      d: 4
    multi:
      <<: *b1
      <<: *b2
    scalar:
      v: <<
  YAML
}

output "merged" {
  value = jsonencode(yamldecode(local.doc)["result"])
}

output "multi" {
  value = jsonencode(yamldecode(local.doc)["multi"])
}

output "scalar_v" {
  value = yamldecode(local.doc)["scalar"]["v"]
}
