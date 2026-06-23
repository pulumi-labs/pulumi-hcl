variable "aMap" {
  type = map(number)
}
output "theMap" {
  value = {
    "a" = var.aMap["a"] + 1
    "b" = var.aMap["b"] + 1
  }
}
variable "anObject" {
  type = object({prop=list(bool)})
}
output "theObject" {
  value = var.anObject.prop[0]
}
variable "anyObject" {
}
output "theThing" {
  value = var.anyObject.a + var.anyObject.b
}
variable "optionalUntypedObject" {
  default = {
    "key" = "value"
  }
}
output "defaultUntypedObject" {
  value = var.optionalUntypedObject
}
variable "optionalList" {
  type    = list(string)
  default = null
}
variable "optionalMap" {
  type    = map(string)
  default = null
}
variable "optionalObject" {
  type    = object({other=number, prop=string})
  default = null
}
output "optionalList" {
  value = var.optionalList == null ? "null" : jsonencode(var.optionalList)
}
output "optionalMap" {
  value = var.optionalMap == null ? "null" : jsonencode(var.optionalMap)
}
output "optionalObject" {
  value = var.optionalObject == null ? "null" : jsonencode(var.optionalObject)
}
