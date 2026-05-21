terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_container" "source" {
  inputs = ["a", "b"]
}
resource "nestedobject_container" "sink" {
  inputs = nestedobject_container.source.details[*].value
}
