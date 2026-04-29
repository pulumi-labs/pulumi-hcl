variable "a" {
  type = number
}
variable "b" {
  type = number
}
variable "c" {
  type = number
}
variable "d" {
  type = number
}
output "maxResult" {
  value = max(var.a, var.b)
}
output "minResult" {
  value = min(var.a, var.b)
}
output "intMaxResult" {
  value = max(var.c, var.d)
}
output "intMinResult" {
  value = min(var.c, var.d)
}
