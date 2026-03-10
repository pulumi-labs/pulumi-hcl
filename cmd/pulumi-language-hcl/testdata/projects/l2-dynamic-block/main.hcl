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
