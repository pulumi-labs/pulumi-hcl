terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_my_invoke" "invoke_0" {
  value = simple-invoke_string_resource.res.text
}
data "simple-invoke_unit" "invoke_1" {
}

resource "simple-invoke_string_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  text = "hello"
}
output "outputInput" {
  value = data.simple-invoke_my_invoke.invoke_0.result
}
output "unit" {
  value = data.simple-invoke_unit.invoke_1.result
}
