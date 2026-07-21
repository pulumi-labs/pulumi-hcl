resource "blocky_thing" "from_dynamic" {
  tag {
    key   = "ov"
    value = "v"
  }
}

resource "blocky_thing" "from_static" {
  dynamic "tag" {
    for_each = ["ov1", "ov2"]
    content {
      key   = tag.value
      value = "v"
    }
  }
}
