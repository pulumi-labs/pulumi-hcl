resource "numResource" "nestedobject:index:Target" {
  name = "num-${range.value}"
  options {
    range = numItems
  }
}

resource "listResource" "nestedobject:index:Target" {
  name = "${range.key}:${range.value}"
  options {
    range = itemList
  }
}

resource "mapResource" "nestedobject:index:Target" {
  name = "${range.key}=${range.value}"
  options {
    range = itemMap
  }
}

resource "boolResource" "nestedobject:index:Target" {
  name = "bool-resource"
  options {
    range = createBool
  }
}

config "numItems" "number" {
}

config "itemList" "list(string)" {
}

config "itemMap" "map(string)" {
}

config "createBool" "bool" {
}

