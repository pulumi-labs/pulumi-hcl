variable "input" {
  type = string
}
locals {
  hash = sha1(var.input)
}
output "hash" {
  value = local.hash
}
