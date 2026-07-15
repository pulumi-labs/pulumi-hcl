terraform {
  required_providers {
    nestedobject = {
      source  = "pulumi/nestedobject"
      version = "1.42.0"
    }
  }
}

resource "nestedobject_target" "item" {
  count = 2
  pulumi {
    name ="item-${count.index}"
  }
  lifecycle {
    create_before_destroy = true
  }
  name ="${var.prefix}-${count.index}"
}
variable "prefix" {
  type = string
}
