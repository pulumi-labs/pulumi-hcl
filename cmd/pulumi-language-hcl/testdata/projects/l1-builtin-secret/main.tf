variable "aSecret" {
  type = string
}
variable "notSecret" {
  type = string
}
output "roundtripSecret" {
  value = var.aSecret
}
output "roundtripNotSecret" {
  value = var.notSecret
}
output "double" {
  value = sensitive(var.aSecret)
}
output "open" {
  value = nonsensitive(var.aSecret)
}
output "close" {
  value = sensitive(var.notSecret)
}
