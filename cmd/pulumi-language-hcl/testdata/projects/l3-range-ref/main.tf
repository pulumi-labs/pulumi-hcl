terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_target" "numResource" {
  count = var.numItems
  name  ="num-${count.index}"
}
resource "nestedobject_target" "numTarget" {
  name ="${nestedobject_target.numResource[0].name}+"
}
resource "nestedobject_target" "listResource" {
  for_each = {  for  __key,  __value  in  var.itemList  :  tostring(__key)  =>  __value  }
  name     ="${each.key}:${each.value}"
}
resource "nestedobject_target" "listTarget" {
  name ="${nestedobject_target.listResource[1].name}+"
}
resource "nestedobject_target" "mapResource" {
  for_each = var.itemMap
  name     ="${each.key}=${each.value}"
}
resource "nestedobject_target" "mapTarget" {
  name ="${nestedobject_target.mapResource["k1"].name}+"
}
resource "nestedobject_target" "boolResource" {
  count = var.createBool
  name  = "bool-resource"
}
resource "nestedobject_target" "boolTarget" {
  name ="${nestedobject_target.boolResource.name}+"
}
variable "numItems" {
  type = number
}
variable "itemList" {
  type = list(string)
}
variable "itemMap" {
  type = map(string)
}
variable "createBool" {
  type = bool
}
