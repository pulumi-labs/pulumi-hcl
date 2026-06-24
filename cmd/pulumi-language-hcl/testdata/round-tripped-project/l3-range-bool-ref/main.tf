terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_target" "boolResource" {
  count = var.createBool
  lifecycle {
    create_before_destroy = true
  }
  name = "bool-resource"
}
resource "nestedobject_target" "boolTarget" {
  lifecycle {
    create_before_destroy = true
  }
  name ="${nestedobject_target.boolResource.name}+"
}
variable "createBool" {
  type = bool
}
