terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

resource "pulumi_providers_simple" "prov" {
}
resource "simple_resource" "res" {
  provider = pulumi_providers_simple.prov
  value    = true
}
