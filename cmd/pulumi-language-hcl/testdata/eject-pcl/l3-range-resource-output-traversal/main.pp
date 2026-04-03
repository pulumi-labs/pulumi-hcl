resource "container" "nestedobject:index:Container" {
  inputs = ["alpha", "bravo"]
}

resource "mapContainer" "nestedobject:index:MapContainer" {
  tags = {
    "k1" = "charlie"
    "k2" = "delta"
  }
}

resource "listOutput" "nestedobject:index:Target" {
  name = range.value.value
  options {
    range = container.details
  }
}

resource "mapOutput" "nestedobject:index:Target" {
  name = "${range.key}=>${range.value}"
  options {
    range = mapContainer.tags
  }
}

