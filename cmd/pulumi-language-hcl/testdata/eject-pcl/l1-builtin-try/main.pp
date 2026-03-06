config "aMap" "map(string)" {
}

output "plainTrySuccess" {
  value = try(aMap["a"], "fallback")
}

output "plainTryFailure" {
  value = try(aMap["b"], "fallback")
}

aSecretMap = secret(aMap)

output "outputTrySuccess" {
  value = try(aSecretMap["a"], "fallback")
}

output "outputTryFailure" {
  value = try(aSecretMap["b"], "fallback")
}

config "anObject" {
}

output "dynamicTrySuccess" {
  value = try(anObject.a, "fallback")
}

output "dynamicTryFailure" {
  value = try(anObject.b, "fallback")
}

aSecretObject = secret(anObject)

output "outputDynamicTrySuccess" {
  value = try(aSecretObject.a, "fallback")
}

output "outputDynamicTryFailure" {
  value = try(aSecretObject.b, "fallback")
}

output "plainTryNull" {
  value = [try(anObject.opt, "fallback")]
}

output "outputTryNull" {
  value = [try(aSecretObject.opt, "fallback")]
}

