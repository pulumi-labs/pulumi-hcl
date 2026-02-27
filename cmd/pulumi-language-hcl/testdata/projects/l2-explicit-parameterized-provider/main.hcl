terraform {
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
resource "goodbye_goodbye" "res" {
  provider = pulumi_providers_goodbye.prov
}
output "parameterValue" {
  value = goodbye_goodbye.res.parameter_value
}
