terraform {
  required_providers {
    simple = {
      source  = "pulumi/simple"
      version = "2.0.0"
    }
  }
}

provider "simple" {
  alias = "prov"
}
resource "simple_resource" "res" {
  provider = simple.prov
  value    = true
}
