pulumi {
  required_providers {
    simple-invoke = {
      source  = "pulumi/simple-invoke"
      version = "10.0.0"
    }
  }
}

data "simple-invoke_my_invoke" "data" {
  value      = "hello"
  depends_on = [simple-invoke_string_resource.first]
}

resource "pulumi_providers_simple-invoke" "explicitProvider" {
}
resource "simple-invoke_string_resource" "first" {
  text = "first hello"
}
resource "simple-invoke_string_resource" "second" {
  text = data.simple-invoke_my_invoke.data.result
}
output "hello" {
  value = data.simple-invoke_my_invoke.data.result
}
