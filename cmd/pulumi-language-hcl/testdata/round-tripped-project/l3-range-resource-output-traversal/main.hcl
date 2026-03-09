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
resource "nestedobject_target" "target" {
  for_each = {  for  __key,  __value  in  nestedobject_container.container.details  :  tostring(__key)  =>  __value  }
  name     = each.value.value
}
