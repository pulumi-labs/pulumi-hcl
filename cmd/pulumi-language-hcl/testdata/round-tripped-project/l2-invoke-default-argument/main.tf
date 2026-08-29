terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_invoke_with_default" "invoke_0" {
}
data "simple-invoke_invoke_with_default" "invoke_1" {
  value = "explicit"
}

output "result" {
  value = data.simple-invoke_invoke_with_default.invoke_0.result
}
output "explicitResult" {
  value = data.simple-invoke_invoke_with_default.invoke_1.result
}
