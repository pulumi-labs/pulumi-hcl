# A `dynamic "tag"` block followed by a static `tag` block for the same
# order-significant TypeList attribute. tofu expands blocks in source order,
# so the provider receives [d1, d2, s1]. pulumi-hcl processes static and
# dynamic blocks in separate passes and always places the static block first
# (or drops the dynamic blocks entirely), losing the source ordering.
resource "blocky_thing" "x" {
  name = "y"

  dynamic "tag" {
    for_each = ["d1", "d2"]
    content {
      key   = tag.value
      value = "v"
    }
  }

  tag {
    key   = "s1"
    value = "v"
  }
}

output "tag_keys" { value = [for t in blocky_thing.x.tag : t.key] }
