terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_myinvoke" "invoke_0" {
  value = simple-invoke_stringresource.res.text
}
data "simple-invoke_unit" "invoke_1" {
}

resource "simple-invoke_stringresource" "res" {
  text = "hello"
}
output "outputInput" {
  value = data.simple-invoke_myinvoke.invoke_0.result
}
output "unit" {
  value = data.simple-invoke_unit.invoke_1.result
}
