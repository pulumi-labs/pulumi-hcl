resource "mapResource" "nestedobject:index:Target" {
  name = "${range.key}=${range.value}"
  options {
    range = itemMap
  }
}

resource "mapTarget" "nestedobject:index:Target" {
  name = "${mapResource["k1"].name}+"
}

config "itemMap" "map(string)" {
}

