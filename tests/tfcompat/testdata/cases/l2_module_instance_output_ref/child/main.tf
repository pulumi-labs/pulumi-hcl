variable "key" {
  type = string
}

resource "order_resource" "a" {
  name         = "a-${var.key}"
  delay_create = var.key == "y"
}

output "result" {
  value = order_resource.a.result
}
