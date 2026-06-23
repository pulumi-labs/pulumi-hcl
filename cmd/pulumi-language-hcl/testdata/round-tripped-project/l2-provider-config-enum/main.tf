terraform {
  required_providers {
    config-enum = {
      source  = "pulumi/config-enum"
      version = "41.0.0"
    }
  }
}

provider "config-enum" {
  alias    = "prov"
  a_string = "hello"
  a_enum   = "two"
}
# Reference the provider's outputs - including the enum - from another resource.
resource "config-enum_resource" "res" {
  provider = config-enum.prov
  lifecycle {
    create_before_destroy = true
  }
  the_string = config-enum.prov.a_string
  the_enum   = config-enum.prov.a_enum
}
