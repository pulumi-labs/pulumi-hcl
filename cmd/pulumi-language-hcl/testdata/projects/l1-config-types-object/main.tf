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
