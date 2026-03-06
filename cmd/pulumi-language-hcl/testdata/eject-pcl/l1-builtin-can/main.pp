config "aMap" "map(string)" {
}

output "plainTrySuccess" {
  value = can(aMap["a"])
}

output "plainTryFailure" {
  value = can(aMap["b"])
}

aSecretMap = secret(aMap)

output "outputTrySuccess" {
  value = can(aSecretMap["a"])
}

output "outputTryFailure" {
  value = can(aSecretMap["b"])
}

config "anObject" {
}

output "dynamicTrySuccess" {
  value = can(anObject.a)
}

output "dynamicTryFailure" {
  value = can(anObject.b)
}

aSecretObject = secret(anObject)

output "outputDynamicTrySuccess" {
  value = can(aSecretObject.a)
}

output "outputDynamicTryFailure" {
  value = can(aSecretObject.b)
}

output "plainTryNull" {
  value = can(anObject.opt)
}

output "outputTryNull" {
  value = can(aSecretObject.opt)
}

