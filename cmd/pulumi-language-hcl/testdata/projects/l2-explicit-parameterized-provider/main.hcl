pulumi {
  required_providers {
    goodbye = {
      source  = "pulumi/goodbye"
      version = "2.0.0"
    }
  }
}

resource "pulumi_providers_goodbye" "prov" {
  text = "World"
}
// The resource name is based on the parameter value
resource "goodbye_goodbye" "res" {
  provider = pulumi_providers_goodbye.prov
}
// The resource name is based on the parameter value and the provider config
output "parameterValue" {
  value = goodbye_goodbye.res.parameter_value
}
