variable "aNumber" {
  type = number
}
output "theNumber" {
  value = var.aNumber + 1.25
}
variable "optionalNumber" {
  type    = number
  default = 41.5
}
output "defaultNumber" {
  value = var.optionalNumber + 1.2
}
variable "anInt" {
  type = number
}
output "theInteger" {
  value = var.anInt + 4
}
variable "optionalInt" {
  type    = number
  default = 1
}
output "defaultInteger" {
  value = var.optionalInt + 2
}
variable "aString" {
  type = string
}
output "theString" {
  value ="${var.aString} World"
}
variable "optionalString" {
  type    = string
  default = "defaultStringValue"
}
output "defaultString" {
  value = var.optionalString
}
variable "aBool" {
  type = bool
}
output "theBool" {
  value = ! var.aBool && true
}
variable "optionalBool" {
  type    = bool
  default = false
}
output "defaultBool" {
  value = var.optionalBool
}
