resource "numResource" "nestedobject:index:Target" {
  name = "num-${range.value}"
  options {
    range = numItems
  }
}

resource "numTarget" "nestedobject:index:Target" {
  name = "${numResource[0].name}+"
}

resource "listResource" "nestedobject:index:Target" {
  name = "${range.key}:${range.value}"
  options {
    range = itemList
  }
}

resource "listTarget" "nestedobject:index:Target" {
  name = "${listResource[1].name}+"
}

resource "mapResource" "nestedobject:index:Target" {
  name = "${range.key}=${range.value}"
  options {
    range = itemMap
  }
}

resource "mapTarget" "nestedobject:index:Target" {
  name = "${mapResource["k1"].name}+"
}

resource "boolResource" "nestedobject:index:Target" {
  name = "bool-resource"
  options {
    range = createBool
  }
}

resource "boolTarget" "nestedobject:index:Target" {
  name = "${boolResource.name}+"
}

config "numItems" "number" {
}

config "itemList" "list(string)" {
}

config "itemMap" "map(string)" {
}

config "createBool" "bool" {
}

