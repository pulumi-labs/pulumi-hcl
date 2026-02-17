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
  value          = "hello"
  secretResponse = simple_resource.first.value
}

resource "simple_resource" "first" {
  value = false
}
resource "simple_resource" "second" {
  value = data.simple-invoke_secretinvoke.invoke_0.secret
}
