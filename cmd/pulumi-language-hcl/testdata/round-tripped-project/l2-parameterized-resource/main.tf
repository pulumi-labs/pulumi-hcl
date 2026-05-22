terraform {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

// The resource name is based on the parameter value
resource "subpackage_helloworld" "example" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "subpackage_helloworldcomponent" "exampleComponent" {
  lifecycle {
    create_before_destroy = true
  }
}
output "parameterValue" {
  value = subpackage_helloworld.example.parameter_value
}
output "parameterValueFromComponent" {
  value = subpackage_helloworldcomponent.exampleComponent.parameter_value
}
