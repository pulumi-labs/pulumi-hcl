resource "container" "nestedobject:index:Container" {
  inputs = ["alpha", "bravo"]
}

resource "target" "nestedobject:index:Target" {
  name = range.value.value
  options {
    range = container.details
  }
}

