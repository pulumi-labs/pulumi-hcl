terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_container" "source" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = ["a", "b", "c"]
}
# for over list<object> output
resource "nestedobject_receiver" "receiver" {
  lifecycle {
    create_before_destroy = true
  }
  dynamic "details" {
    for_each = nestedobject_container.source.details
    content {
      key   = details.value.key
      value = details.value.value
    }
  }
}
# for over list<string> output
resource "nestedobject_container" "fromSimple" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = [for _, detail in nestedobject_container.source.details : detail.value]
}
# for producing a map
resource "nestedobject_map_container" "mapped" {
  lifecycle {
    create_before_destroy = true
  }
  tags = {for _, detail in nestedobject_container.source.details : detail.key => detail.value}
}
