terraform {
  required_providers {
    simple-invoke-with-scalar-return = {
      source  = "pulumi/simple-invoke-with-scalar-return"
      version = "17.0.0"
    }
  }
}

data "simple-invoke-with-scalar-return_myinvokescalar" "invoke_0" {
  value = "goodbye"
}

output "scalar" {
  value = data.simple-invoke-with-scalar-return_myinvokescalar.invoke_0
}
