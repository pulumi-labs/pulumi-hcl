terraform {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

data "subpackage_do_hello_world" "invoke_0" {
  input = "goodbye"
}

// The invoke name is based on the parameter value
output "parameterValue" {
  value = data.subpackage_do_hello_world.invoke_0.output
}
