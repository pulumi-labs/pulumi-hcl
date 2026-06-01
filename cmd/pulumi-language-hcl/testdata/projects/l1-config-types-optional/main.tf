variable "names" {
  type    = list(string)
  default = [null, "hello", null]
}
output "namesLength" {
  value = length(var.names)
}
