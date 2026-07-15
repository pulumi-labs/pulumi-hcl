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
resource "nestedobject_target" "numTarget" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${nestedobject_target.numResource[0].name}+"
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
resource "nestedobject_target" "listTarget" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${nestedobject_target.listResource[1].name}+"
}
resource "nestedobject_target" "listDynTarget" {
  for_each = {  for  __key,  __value  in  var.itemList  :  tostring(__key)  =>  __value  }
  pulumi {
    name ="listDynTarget-${each.key}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="${nestedobject_target.listResource[each.key].name}!"
}
variable "numItems" {
  type = number
}
variable "itemList" {
  type = list(string)
}
