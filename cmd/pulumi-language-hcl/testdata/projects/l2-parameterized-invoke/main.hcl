terraform {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

data "subpackage_dohelloworld" "invoke_0" {
  input = "goodbye"
}

output "parameterValue" {
  value = data.subpackage_dohelloworld.invoke_0.output
}
