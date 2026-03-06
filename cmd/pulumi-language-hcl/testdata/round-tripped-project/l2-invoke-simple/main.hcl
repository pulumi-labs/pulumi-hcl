terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_myinvoke" "invoke_0" {
  value = "hello"
}
data "simple-invoke_myinvoke" "invoke_1" {
  value = "goodbye"
}

output "hello" {
  value = data.simple-invoke_myinvoke.invoke_0.result
}
output "goodbye" {
  value = data.simple-invoke_myinvoke.invoke_1.result
}
