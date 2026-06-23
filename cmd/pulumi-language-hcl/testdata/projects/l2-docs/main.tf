terraform {
  required_providers {
    docs = {
      source  = "pulumi/docs"
      version = "28.0.0"
    }
    enum = {
      source  = "pulumi/enum"
      version = "30.0.0"
    }
  }
}

data "docs_fun" "invoke_0" {
  in = false
}

resource "enum_res" "enumRes" {
  lifecycle {
    create_before_destroy = true
  }
  int_enum    = 1
  string_enum = "one"
}
resource "docs_resource" "res" {
  lifecycle {
    create_before_destroy = true
  }
  in            = data.docs_fun.invoke_0.out
  external_enum = "one"
}
