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
  inputs = ["a", "b"]
}
resource "nestedobject_container" "sink" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = nestedobject_container.source.details[*].value
}
