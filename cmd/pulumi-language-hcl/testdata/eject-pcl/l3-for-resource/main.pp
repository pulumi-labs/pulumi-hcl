resource "source" "nestedobject:index:Container" {
  inputs = ["a", "b", "c"]
}

resource "receiver" "nestedobject:index:Receiver" {
  details = [for __key, __value in source.details : {
    key   = __value.key
    value = __value.value
  }]
}

resource "fromSimple" "nestedobject:index:Container" {
  inputs = [for _, detail in source.details : detail.value]
}

resource "mapped" "nestedobject:index:MapContainer" {
  tags = {for _, detail in source.details : detail.key => detail.value}
}

