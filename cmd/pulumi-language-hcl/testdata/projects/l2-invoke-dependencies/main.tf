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

data "simple-invoke_secret_invoke" "invoke_0" {
  value           = "hello"
  secret_response = simple_resource.first.value
}
data "simple-invoke_text" "data" {
  text = simple-invoke_string_resource.third.text
}

resource "simple_resource" "first" {
  lifecycle {
    create_before_destroy = true
  }
  value = false
}
// assert that resource second depends on resource first
// because it uses .secret from the invoke which depends on first
resource "simple_resource" "second" {
  lifecycle {
    create_before_destroy = true
  }
  value = data.simple-invoke_secret_invoke.invoke_0.secret
}
resource "simple-invoke_string_resource" "third" {
  lifecycle {
    create_before_destroy = true
  }
  text = "third"
}
resource "simple-invoke_string_resource" "fourth" {
  lifecycle {
    create_before_destroy = true
  }
  text = data.simple-invoke_text.data.result
}
// third.text is known during preview, but third does not exist yet. SDKs must
// infer the dependency on third from the invoke's arguments and skip the
// invoke while third's ID is unknown: getText fails if it is called before
// third has been created.
