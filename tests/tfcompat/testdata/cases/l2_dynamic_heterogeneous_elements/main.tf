# A `dynamic` block whose `for_each` source has elements that set different
# subsets of the same optional keys. After `lookup(..., null)` runs, the
# per-element object types diverge (one has `note: string`, the other has
# `note: null` of dynamic type). tofu unifies and plans cleanly; pulumi-hcl
# previously panicked inside cty.ListVal with "inconsistent list element
# types".
locals {
  tags = [
    { key = "a", value = "1", note = "noted" },
    { key = "b", value = "2" },
  ]
}

resource "blocky_thing" "t" {
  name = "het"

  dynamic "tag" {
    for_each = local.tags
    content {
      key   = tag.value.key
      value = tag.value.value
      note  = lookup(tag.value, "note", null)
    }
  }
}

output "summary" {
  value = blocky_thing.t.summary
}
