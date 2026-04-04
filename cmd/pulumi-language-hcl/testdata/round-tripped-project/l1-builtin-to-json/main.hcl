variable "aString" {
  type = string
}
variable "aNumber" {
  type = number
}
variable "aList" {
  type = list(string)
}
variable "aSecret" {
  type = string
}
output "stringOutput" {
  value = jsonencode(var.aString)
}
output "numberOutput" {
  value = jsonencode(var.aNumber)
}
output "boolOutput" {
  value = jsonencode(true)
}
output "arrayOutput" {
  value = jsonencode(["x", "y", "z"])
}
output "objectOutput" {
  value = jsonencode({
    "key"   = "value"
    "count" = 1
  })
}
locals {
  nestedObject = {
    "anObject" = {
      "name"  = var.aString
      "items" = var.aList
    }
    "a_secret" = var.aSecret
  }
}
output "nestedOutput" {
  value = jsonencode(local.nestedObject)
}
