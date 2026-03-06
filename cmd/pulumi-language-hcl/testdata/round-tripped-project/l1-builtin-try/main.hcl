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
variable "anObject" {
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
output "plainTryNull" {
  value = [try(var.anObject.opt, "fallback")]
}
output "outputTryNull" {
  value = [try(local.aSecretObject.opt, "fallback")]
}
