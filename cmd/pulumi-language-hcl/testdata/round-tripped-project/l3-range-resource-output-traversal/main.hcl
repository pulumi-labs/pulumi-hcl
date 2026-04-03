terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_container" "container" {
  inputs = ["alpha", "bravo"]
}
resource "nestedobject_mapcontainer" "mapContainer" {
  tags = {
    "k1" = "charlie"
    "k2" = "delta"
  }
}
resource "nestedobject_target" "listOutput" {
  for_each = {  for  __key,  __value  in  nestedobject_container.container.details  :  tostring(__key)  =>  __value  }
  name     = each.value.value
}
resource "nestedobject_target" "mapOutput" {
  for_each = nestedobject_mapcontainer.mapContainer.tags
  name     ="${each.key}=>${each.value}"
}
