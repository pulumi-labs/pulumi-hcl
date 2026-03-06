terraform {
  required_providers {
    camelNames = {
      source  = "pulumi/camelNames"
      version = "19.0.0"
    }
  }
}

resource "camelNames_coolmodule_someresource" "firstResource" {
  the_input = true
}
resource "camelNames_coolmodule_someresource" "secondResource" {
  the_input = camelNames_coolmodule_someresource.firstResource.the_output
}
resource "camelNames_coolmodule_someresource" "thirdResource" {
  the_input     = true
  resource_name = "my-cluster"
}
