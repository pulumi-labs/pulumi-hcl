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

resource "listDynTarget" "nestedobject:index:Target" {
  name = "${listResource[range.key].name}!"
  options {
    range = itemList
  }
}

config "numItems" "number" {
}

config "itemList" "list(string)" {
}

