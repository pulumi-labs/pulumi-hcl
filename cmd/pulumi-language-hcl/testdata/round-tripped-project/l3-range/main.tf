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
  pulumi {
    name ="numResource-${count.index}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="num-${count.index}"
}
resource "nestedobject_target" "listResource" {
  for_each = {  for  __key,  __value  in  var.itemList  :  tostring(__key)  =>  __value  }
  pulumi {
    name ="listResource-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="${each.key}:${each.value}"
}
resource "nestedobject_target" "mapResource" {
  for_each = var.itemMap
  pulumi {
    name ="mapResource-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="${each.key}=${each.value}"
}
resource "nestedobject_target" "boolResource" {
  count = var.createBool
  lifecycle {
    create_before_destroy = true
  }
  name = "bool-resource"
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
