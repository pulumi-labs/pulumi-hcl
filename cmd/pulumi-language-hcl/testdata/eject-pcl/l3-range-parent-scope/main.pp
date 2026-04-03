resource "item" "nestedobject:index:Target" {
  name = "${prefix}-${range.value}"
  options {
    range = 2
  }
}

config "prefix" "string" {
}

