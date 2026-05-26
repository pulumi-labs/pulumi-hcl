# `dynamic` block with an explicit `iterator = <name>` renames the
# iteration variable from the default (block label) to the given
# identifier. Pulumi-hcl was evaluating that identifier as a normal
# expression — and failing because the name isn't bound.
resource "blocky_thing" "x" {
  name = "y"

  dynamic "tag" {
    for_each = ["one", "two"]
    iterator = pblock
    content {
      key   = "k-${pblock.value}"
      value = pblock.value
    }
  }
}

output "summary" { value = blocky_thing.x.summary }
