terraform {
  required_providers {
    goodbye = {
      source  = "pulumi/goodbye"
      version = "2.0.0"
    }
  }
}

provider "goodbye" {
  alias = "prov"
  text  = "World"
}
// The resource name is based on the parameter value
resource "goodbye_goodbye" "res" {
  provider = goodbye.prov
}
// The resource name is based on the parameter value and the provider config
output "parameterValue" {
  value = goodbye_goodbye.res.parameter_value
}
