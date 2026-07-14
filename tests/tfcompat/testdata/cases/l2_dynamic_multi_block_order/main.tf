# Two `dynamic "tag"` blocks for the same repeating (TypeList) block. tofu
# expands them in source order, so the provider receives the tags as
# [a1, a2, b1, b2]. pulumi-hcl merges a later dynamic block ahead of the
# earlier one, so the provider instead receives [b1, b2, a1, a2] — the block
# groups are reversed. `tag` is order-significant (TypeList), so this is an
# observable divergence even though blocky's summary sorts the tags.
resource "blocky_thing" "x" {
  name = "y"

  dynamic "tag" {
    for_each = ["a1", "a2"]
    content {
      key   = tag.value
      value = "v"
    }
  }

  dynamic "tag" {
    for_each = ["b1", "b2"]
    content {
      key   = tag.value
      value = "v"
    }
  }
}

output "summary" { value = blocky_thing.x.summary }
