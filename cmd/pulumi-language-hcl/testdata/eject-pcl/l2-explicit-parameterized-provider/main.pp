package "goodbye" {
  baseProviderName    = "parameterized"
  baseProviderVersion = "1.2.3"
  parameterization {
    name    = "goodbye"
    version = "2.0.0"
    value   = "R29vZGJ5ZQ=="
  }
}

resource "prov" "pulumi:providers:goodbye" {
  text = "World"
}

resource "res" "goodbye:index:Goodbye" {
  options {
    provider = prov
  }
}

output "parameterValue" {
  value = res.parameterValue
}

