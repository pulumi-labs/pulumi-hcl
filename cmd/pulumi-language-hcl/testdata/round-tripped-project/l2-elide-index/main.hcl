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

data "simple-invoke_myinvoke" "invoke_0" {
  value = "test"
}

resource "simple_resource" "res" {
  value = true
}
output "inv" {
  value = data.simple-invoke_myinvoke.invoke_0.result
}
