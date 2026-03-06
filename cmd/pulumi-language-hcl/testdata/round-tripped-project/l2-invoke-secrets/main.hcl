terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_secretinvoke" "invoke_0" {
  value           = "hello"
  secret_response = false
}
data "simple-invoke_secretinvoke" "invoke_1" {
  value           = "hello"
  secret_response = simple_resource.res.value
}
data "simple-invoke_secretinvoke" "invoke_2" {
  value           = sensitive("goodbye")
  secret_response = false
}

resource "simple_resource" "res" {
  value = true
}
output "nonSecret" {
  value = data.simple-invoke_secretinvoke.invoke_0.response
}
output "firstSecret" {
  value = data.simple-invoke_secretinvoke.invoke_1.response
}
output "secondSecret" {
  value = data.simple-invoke_secretinvoke.invoke_2.response
}
