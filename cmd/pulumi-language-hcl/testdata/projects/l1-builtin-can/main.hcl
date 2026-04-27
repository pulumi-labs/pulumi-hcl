variable "aMap" {
  type = map(string)
}
output "plainTrySuccess" {
  value = can(var.aMap["a"])
}
output "plainTryFailure" {
  value = can(var.aMap["b"])
}
locals {
  aSecretMap = sensitive(var.aMap)
}
output "outputTrySuccess" {
  value = can(local.aSecretMap["a"])
}
output "outputTryFailure" {
  value = can(local.aSecretMap["b"])
}
# A dynamically typed value, whose field accesses will not be type errors (since the type is not known to the type
# checker), but may fail dynamically, and can thus be used as test inputs to can.
variable "anObject" {
}
output "dynamicTrySuccess" {
  value = can(var.anObject.a)
}
output "dynamicTryFailure" {
  value = can(var.anObject.b)
}
locals {
  aSecretObject = sensitive(var.anObject)
}
output "outputDynamicTrySuccess" {
  value = can(local.aSecretObject.a)
}
output "outputDynamicTryFailure" {
  value = can(local.aSecretObject.b)
}
# Check that explicit null values can be returned
output "plainTryNull" {
  value = can(var.anObject.opt)
}
output "outputTryNull" {
  value = can(local.aSecretObject.opt)
}
