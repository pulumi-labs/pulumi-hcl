terraform {
  required_providers {
    scalar-returns = {
      source  = "pulumi/scalar-returns"
      version = "21.0.0"
    }
  }
}

data "scalar-returns_invokesecret" "invoke_0" {
  value = "goodbye"
}
data "scalar-returns_invokearray" "invoke_1" {
  value = "the word"
}
data "scalar-returns_invokemap" "invoke_2" {
  value = "hello"
}
data "scalar-returns_invokemap" "invoke_3" {
  value = "secret"
}

output "secret" {
  value = data.scalar-returns_invokesecret.invoke_0
}
output "array" {
  value = data.scalar-returns_invokearray.invoke_1
}
output "map" {
  value = data.scalar-returns_invokemap.invoke_2
}
output "secretMap" {
  value = data.scalar-returns_invokemap.invoke_3
}
