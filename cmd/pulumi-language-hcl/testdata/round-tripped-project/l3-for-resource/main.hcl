terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_container" "source" {
  inputs = ["a", "b", "c"]
}
resource "nestedobject_receiver" "receiver" {
  dynamic "details" {
    for_each = nestedobject_container.source.details
    content {
      key   = details.value.key
      value = details.value.value
    }
  }
}
resource "nestedobject_container" "fromSimple" {
  inputs = [for _, detail in nestedobject_container.source.details : detail.value]
}
resource "nestedobject_mapcontainer" "mapped" {
  tags = {for _, detail in nestedobject_container.source.details : detail.key => detail.value}
}
