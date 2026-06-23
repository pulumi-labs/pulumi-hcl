terraform {
  required_providers {
    multi-argument-invoke = {
      source  = "pulumi/multi-argument-invoke"
      version = "44.0.0"
    }
  }
}

data "multi-argument-invoke_multi_argument_invoke" "invoke_0" {
  first  = "hello"
  second = "world"
}
data "multi-argument-invoke_multi_argument_invoke" "invoke_1" {
  first = "hello"
}

output "both" {
  value = data.multi-argument-invoke_multi_argument_invoke.invoke_0.result
}
output "onlyRequired" {
  value = data.multi-argument-invoke_multi_argument_invoke.invoke_1.result
}
