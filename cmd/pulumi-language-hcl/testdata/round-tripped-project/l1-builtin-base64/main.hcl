variable "input" {
  type = string
}
locals {
  bytes = base64decode(var.input)
}
output "data" {
  value = local.bytes
}
output "roundtrip" {
  value = base64encode(local.bytes)
}
