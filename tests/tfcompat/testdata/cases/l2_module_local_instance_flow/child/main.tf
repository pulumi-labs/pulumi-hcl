variable "key" {
  type = string
}

resource "order_resource" "a" {
  name         = "a-${var.key}"
  delay_create = var.key == "y"
}

locals {
  r = order_resource.a.result
}

resource "order_resource" "b" {
  name = "b-${local.r}"
}

output "result" {
  value = order_resource.b.result
}
