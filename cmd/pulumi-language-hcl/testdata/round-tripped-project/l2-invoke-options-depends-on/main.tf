terraform {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_myinvoke" "invoke_0" {
  value      = "hello"
  depends_on = [simple-invoke_stringresource.first]
}
data "simple-invoke_myinvoke" "invoke_1" {
  value      = "hello"
  depends_on = [simple-invoke_stringresource.first]
}

provider "simple-invoke" {
  alias = "explicitProvider"
}
resource "simple-invoke_stringresource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  text = "first hello"
}
resource "simple-invoke_stringresource" "second" {
  lifecycle {
    create_before_destroy = true
  }
  text = data.simple-invoke_myinvoke.invoke_0.result
}
output "hello" {
  value = data.simple-invoke_myinvoke.invoke_1.result
}
