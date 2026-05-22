pulumi {
  required_providers {
    output-only-invoke = {
      source  = "pulumi/output-only-invoke"
      version = "24.0.0"
    }
  }
}

data "output-only-invoke_my_invoke" "invoke_0" {
  value = "hello"
}
data "output-only-invoke_my_invoke" "invoke_1" {
  value = "goodbye"
}

output "hello" {
  value = data.output-only-invoke_my_invoke.invoke_0.result
}
output "goodbye" {
  value = data.output-only-invoke_my_invoke.invoke_1.result
}
