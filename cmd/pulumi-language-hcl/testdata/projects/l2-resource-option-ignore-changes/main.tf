terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_receiver" "receiverIgnore" {
  lifecycle {
    ignore_changes        = [details[0].key]
    create_before_destroy = true
  }
  details {
    key   = "a"
    value = "b"
  }
}
resource "nestedobject_map_container" "mapIgnore" {
  lifecycle {
    ignore_changes        = [tags["env"], tags["with.dot"], tags["with escaped \""]]
    create_before_destroy = true
  }
  tags = {
    "env" = "prod"
  }
}
resource "nestedobject_target" "noIgnore" {
  lifecycle {
    create_before_destroy = true
  }
  name = "nothing"
}
