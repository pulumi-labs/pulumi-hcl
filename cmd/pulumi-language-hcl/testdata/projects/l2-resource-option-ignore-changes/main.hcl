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
    ignore_changes = [details[0].key]
  }
  details {
    key   = "a"
    value = "b"
  }
}
resource "nestedobject_mapcontainer" "mapIgnore" {
  lifecycle {
    ignore_changes = [tags["env"], tags["with.dot"], tags["with escaped \""]]
  }
  tags = {
    "env" = "prod"
  }
}
resource "nestedobject_target" "noIgnore" {
  name = "nothing"
}
