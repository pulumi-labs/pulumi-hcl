pulumi {
  required_providers {
    subpackage = {
      source  = "pulumi/subpackage"
      version = "2.0.0"
    }
  }
}

// The resource name is based on the parameter value
resource "subpackage_hello_world" "example" {
}
resource "subpackage_hello_world_component" "exampleComponent" {
}
output "parameterValue" {
  value = subpackage_hello_world.example.parameter_value
}
output "parameterValueFromComponent" {
  value = subpackage_hello_world_component.exampleComponent.parameter_value
}
