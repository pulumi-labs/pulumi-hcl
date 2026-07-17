variable "key" {
  type = string
}

resource "order_resource" "a" {
  name         = "a-${var.key}"
  delay_create = var.key == "y"
}

resource "order_resource" "b" {
  name = "b-${order_resource.a.result}"
}

output "result" {
  value = order_resource.b.result
}
