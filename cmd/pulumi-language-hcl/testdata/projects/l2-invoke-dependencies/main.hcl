pulumi {
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

data "simple-invoke_secret_invoke" "invoke_0" {
  value           = "hello"
  secret_response = simple_resource.first.value
}

resource "simple_resource" "first" {
  value = false
}
// assert that resource second depends on resource first
// because it uses .secret from the invoke which depends on first
resource "simple_resource" "second" {
  value = data.simple-invoke_secret_invoke.invoke_0.secret
}
