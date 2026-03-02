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
resource "nestedobject_target" "listResource" {
  for_each = {  for  __key,  __value  in  var.itemList  :  tostring(__key)  =>  __value  }
  name     ="${each.key}:${each.value}"
}
resource "nestedobject_target" "mapResource" {
  for_each = var.itemMap
  name     ="${each.key}=${each.value}"
}
resource "nestedobject_target" "boolResource" {
  count = var.createBool
  name  = "bool-resource"
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
