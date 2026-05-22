terraform {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

// The resource name is based on the parameter value
resource "subpackage_hello_world" "example" {
  lifecycle {
    create_before_destroy = true
  }
}
resource "subpackage_hello_world_component" "exampleComponent" {
  lifecycle {
    create_before_destroy = true
  }
}
output "parameterValue" {
  value = subpackage_hello_world.example.parameter_value
}
output "parameterValueFromComponent" {
  value = subpackage_hello_world_component.exampleComponent.parameter_value
}
