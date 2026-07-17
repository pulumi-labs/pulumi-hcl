variable "idx" {
  type = number
}

resource "order_resource" "a" {
  name         = "a-${var.idx}"
  delay_create = var.idx == 1
}

resource "order_resource" "b" {
  name = "b-${order_resource.a.result}"
}

output "result" {
  value = order_resource.b.result
}
