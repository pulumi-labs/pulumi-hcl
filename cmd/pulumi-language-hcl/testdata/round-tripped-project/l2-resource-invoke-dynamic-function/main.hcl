terraform {
  required_providers {
    any-type-function = {
      source  = "pulumi/any-type-function"
      version = "15.0.0"
    }
  }
}

data "any-type-function_dynlisttodyn" "invoke_0" {
  inputs = ["hello", local.localValue, {}]
}

locals {
  localValue = "hello"
}
output "dynamic" {
  value = data.any-type-function_dynlisttodyn.invoke_0.result
}
