pulumi {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_myinvoke" "data" {
  value      = "hello"
  depends_on = [simple-invoke_stringresource.first]
}

resource "pulumi_providers_simple-invoke" "explicitProvider" {
}
resource "simple-invoke_stringresource" "first" {
  text = "first hello"
}
resource "simple-invoke_stringresource" "second" {
  text = data.simple-invoke_myinvoke.data.result
}
output "hello" {
  value = data.simple-invoke_myinvoke.data.result
}
