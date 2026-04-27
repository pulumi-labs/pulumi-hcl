resource "container" "nestedobject:index:Container" {
  inputs = ["alpha", "bravo"]
}

resource "mapContainer" "nestedobject:index:MapContainer" {
  tags = {
    "k1" = "charlie"
    "k2" = "delta"
  }
}

# A resource that ranges over a computed list
resource "listOutput" "nestedobject:index:Target" {
  name = range.value.value
  options {
    range = container.details
  }
}

# A resource that ranges over a computed map
resource "mapOutput" "nestedobject:index:Target" {
  name = "${range.key}=>${range.value}"
  options {
    range = mapContainer.tags
  }
}

