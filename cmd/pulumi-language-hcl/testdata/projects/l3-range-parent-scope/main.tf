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
  lifecycle {
    create_before_destroy = true
  }
  name ="${var.prefix}-${count.index}"
}
variable "prefix" {
  type = string
}
