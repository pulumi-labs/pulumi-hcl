pulumi {
  required_providers {
    camelNames = {
      source  = "pulumi/camelNames"
      version = "19.0.0"
    }
  }
}

resource "camelNames_cool_module_some_resource" "firstResource" {
  the_input = true
}
resource "camelNames_cool_module_some_resource" "secondResource" {
  the_input = camelNames_cool_module_some_resource.firstResource.the_output
}
resource "camelNames_cool_module_some_resource" "thirdResource" {
  the_input     = true
  resource_name = "my-cluster"
}
