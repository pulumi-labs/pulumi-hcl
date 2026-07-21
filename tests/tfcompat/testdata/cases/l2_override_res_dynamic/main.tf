# A dynamic block counts as the type it generates, so the static `tag` block
# in the override hides the base's generated tags, and the dynamic `tag`
# block in the override hides the static ones the base declares.
resource "blocky_thing" "from_dynamic" {
  name = "a"

  dynamic "tag" {
    for_each = ["base1", "base2"]
    content {
      key   = tag.value
      value = "v"
    }
  }
}

resource "blocky_thing" "from_static" {
  name = "b"

  tag {
    key   = "base1"
    value = "v"
  }

  tag {
    key   = "base2"
    value = "v"
  }
}

output "from_dynamic" { value = [for t in blocky_thing.from_dynamic.tag : t.key] }
output "from_static" { value = [for t in blocky_thing.from_static.tag : t.key] }
