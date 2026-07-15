# A three-resource dependency chain base <- middle <- top, where only the top
# declares create_before_destroy. Changing base's ForceNew label replaces all
# three. create_before_destroy must propagate transitively to middle and base,
# so every create runs before any delete and top records witness = 3.
resource "cascade_parent" "base" {
  label = "L2"
}

resource "cascade_parent" "middle" {
  label = cascade_parent.base.result
}

resource "cascade_child" "top" {
  parent = cascade_parent.middle.result

  lifecycle {
    create_before_destroy = true
  }
}

output "witness" {
  value = cascade_child.top.witness
}
