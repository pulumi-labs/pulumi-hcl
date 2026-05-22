terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_container" "container" {
  lifecycle {
    create_before_destroy = true
  }
  inputs = ["alpha", "bravo"]
}
resource "nestedobject_map_container" "mapContainer" {
  lifecycle {
    create_before_destroy = true
  }
  tags = {
    "k1" = "charlie"
    "k2" = "delta"
  }
}
# A resource that ranges over a computed list
resource "nestedobject_target" "listOutput" {
  for_each = {  for  __key,  __value  in  nestedobject_container.container.details  :  tostring(__key)  =>  __value  }
  lifecycle {
    create_before_destroy = true
  }
  name = each.value.value
}
# A resource that ranges over a computed map
resource "nestedobject_target" "mapOutput" {
  for_each = nestedobject_map_container.mapContainer.tags
  lifecycle {
    create_before_destroy = true
  }
  name ="${each.key}=>${each.value}"
}
