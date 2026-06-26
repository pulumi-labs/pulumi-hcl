variable "aMap" {
  type = map(string)
}
output "plainTrySuccess" {
  value = try(var.aMap["a"], "fallback")
}
output "plainTryFailure" {
  value = try(var.aMap["b"], "fallback")
}
locals {
  aSecretMap = sensitive(var.aMap)
}
output "outputTrySuccess" {
  value = try(local.aSecretMap["a"], "fallback")
}
output "outputTryFailure" {
  value = try(local.aSecretMap["b"], "fallback")
}
# A dynamically typed value, whose field accesses will not be type errors (since the type is not known to the type
# checker), but may fail dynamically, and can thus be used as test inputs to try.
variable "anObject" {
  type = any
}
output "dynamicTrySuccess" {
  value = try(var.anObject.a, "fallback")
}
output "dynamicTryFailure" {
  value = try(var.anObject.b, "fallback")
}
locals {
  aSecretObject = sensitive(var.anObject)
}
output "outputDynamicTrySuccess" {
  value = try(local.aSecretObject.a, "fallback")
}
output "outputDynamicTryFailure" {
  value = try(local.aSecretObject.b, "fallback")
}
# Check that explicit null values can be returned.
# It's not safe to return a null value directly (see l1-output-null 
# and https://github.com/pulumi/pulumi/issues/19015) so wrap these in a list.
output "plainTryNull" {
  value = [try(var.anObject.opt, "fallback")]
}
output "outputTryNull" {
  value = [try(local.aSecretObject.opt, "fallback")]
}
