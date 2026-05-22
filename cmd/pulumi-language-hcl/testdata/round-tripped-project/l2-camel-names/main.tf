terraform {
  required_providers {
    camelNames = {
      source  = "pulumi/camelNames"
      version = "19.0.0"
    }
  }
}

resource "camelNames_coolmodule_someresource" "firstResource" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = true
}
resource "camelNames_coolmodule_someresource" "secondResource" {
  lifecycle {
    create_before_destroy = true
  }
  the_input = camelNames_coolmodule_someresource.firstResource.the_output
}
resource "camelNames_coolmodule_someresource" "thirdResource" {
  lifecycle {
    create_before_destroy = true
  }
  the_input     = true
  resource_name = "my-cluster"
}
