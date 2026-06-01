terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_my_invoke" "invoke_0" {
  value      = "hello"
  depends_on = [simple-invoke_string_resource.first]
}
data "simple-invoke_my_invoke" "invoke_1" {
  value      = "hello"
  depends_on = [simple-invoke_string_resource.first]
}

provider "simple-invoke" {
  alias = "explicitProvider"
}
resource "simple-invoke_string_resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  text = "first hello"
}
resource "simple-invoke_string_resource" "second" {
  lifecycle {
    create_before_destroy = true
  }
  text = data.simple-invoke_my_invoke.invoke_0.result
}
output "hello" {
  value = data.simple-invoke_my_invoke.invoke_1.result
}
