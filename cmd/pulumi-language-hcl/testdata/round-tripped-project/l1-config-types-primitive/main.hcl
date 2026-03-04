variable "aNumber" {
  type = number
}
output "theNumber" {
  value = var.aNumber + 1.25
}
variable "aString" {
  type = string
}
output "theString" {
  value ="${var.aString} World"
}
variable "aBool" {
  type = bool
}
output "theBool" {
  value = ! var.aBool && true
}
